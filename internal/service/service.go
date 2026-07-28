package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fndns/manager/internal/id"
	"github.com/fndns/manager/internal/model"
	"github.com/fndns/manager/internal/provider"
	"github.com/fndns/manager/internal/secretbox"
	"github.com/fndns/manager/internal/store"
)

var (
	ErrConflict     = errors.New("远端记录已发生变化，请先刷新后再操作")
	ErrExpiredCache = errors.New("缓存已超过 6 个月，请先刷新后再操作")
)

type Service struct {
	store    *store.Store
	box      *secretbox.Box
	factory  *provider.Factory
	operator string
	mu       sync.Mutex
}

func New(storage *store.Store, box *secretbox.Box, factory *provider.Factory) *Service {
	return &Service{store: storage, box: box, factory: factory, operator: "FNOS 管理员"}
}

func (s *Service) Store() *store.Store { return s.store }

func validateCredentialInput(input model.CredentialInput, requireSecret bool) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return errors.New("凭据名称不能为空")
	}
	if len([]rune(input.Name)) > 60 {
		return errors.New("凭据名称不能超过 60 个字符")
	}
	switch input.Provider {
	case model.ProviderCloudflare:
		if requireSecret && strings.TrimSpace(input.Token) == "" {
			return errors.New("Cloudflare API Token 不能为空")
		}
	case model.ProviderTencent:
		if requireSecret && (strings.TrimSpace(input.SecretID) == "" || strings.TrimSpace(input.SecretKey) == "") {
			return errors.New("腾讯云 SecretId 和 SecretKey 不能为空")
		}
	default:
		return errors.New("请选择受支持的 DNS 平台")
	}
	return nil
}

func secretHint(providerName string, secret model.CredentialSecret) string {
	mask := func(value string) string {
		runes := []rune(value)
		if len(runes) <= 8 {
			return "••••"
		}
		return string(runes[:4]) + "••••" + string(runes[len(runes)-4:])
	}
	if providerName == model.ProviderCloudflare {
		return mask(secret.Token)
	}
	return mask(secret.SecretID)
}

func (s *Service) AddCredential(ctx context.Context, input model.CredentialInput) (model.Credential, error) {
	if err := validateCredentialInput(input, true); err != nil {
		return model.Credential{}, err
	}
	secret := input.Secret()
	remote, err := s.factory.Build(input.Provider, secret)
	if err != nil {
		return model.Credential{}, err
	}
	if err := remote.Validate(ctx); err != nil {
		s.log(ctx, model.AuditLog{Action: "credential.create", Result: "failed", CredentialName: input.Name,
			Provider: input.Provider, Message: err.Error()})
		return model.Credential{}, fmt.Errorf("凭据验证失败: %w", err)
	}
	encrypted, err := s.box.Seal(secret)
	if err != nil {
		return model.Credential{}, err
	}
	now := time.Now()
	stored := model.StoredCredential{Credential: model.Credential{ID: id.New(), Name: strings.TrimSpace(input.Name),
		Provider: input.Provider, SecretHint: secretHint(input.Provider, secret), CreatedAt: now, UpdatedAt: now,
		LastSyncStatus: model.SyncNever}, EncryptedSecret: encrypted}
	if err := s.store.CreateCredential(ctx, stored); err != nil {
		return model.Credential{}, err
	}
	s.log(ctx, model.AuditLog{Action: "credential.create", Result: "success", CredentialID: stored.ID,
		CredentialName: stored.Name, Provider: stored.Provider, Message: "API 凭据已验证并加密保存"})
	if err := s.refreshCredentialWith(ctx, stored, remote); err != nil {
		latest, readErr := s.store.GetCredential(ctx, stored.ID)
		if readErr == nil {
			return latest.Credential, nil
		}
		return stored.Credential, nil
	}
	latest, err := s.store.GetCredential(ctx, stored.ID)
	return latest.Credential, err
}

func (s *Service) UpdateCredential(ctx context.Context, credentialID string, input model.CredentialInput) (model.Credential, error) {
	stored, err := s.store.GetCredential(ctx, credentialID)
	if err != nil {
		return model.Credential{}, err
	}
	if input.Provider == "" {
		input.Provider = stored.Provider
	}
	if err := validateCredentialInput(input, false); err != nil {
		return model.Credential{}, err
	}
	var secret model.CredentialSecret
	if err := s.box.Open(stored.EncryptedSecret, &secret); err != nil {
		return model.Credential{}, err
	}
	if input.Provider != stored.Provider {
		if err := validateCredentialInput(input, true); err != nil {
			return model.Credential{}, err
		}
		secret = input.Secret()
	} else if input.Provider == model.ProviderCloudflare {
		if strings.TrimSpace(input.Token) != "" {
			secret.Token = strings.TrimSpace(input.Token)
		}
	} else {
		if strings.TrimSpace(input.SecretID) != "" {
			secret.SecretID = strings.TrimSpace(input.SecretID)
		}
		if strings.TrimSpace(input.SecretKey) != "" {
			secret.SecretKey = strings.TrimSpace(input.SecretKey)
		}
	}
	remote, err := s.factory.Build(input.Provider, secret)
	if err != nil {
		return model.Credential{}, err
	}
	if err := remote.Validate(ctx); err != nil {
		s.log(ctx, model.AuditLog{Action: "credential.update", Result: "failed", CredentialID: stored.ID,
			CredentialName: stored.Name, Provider: stored.Provider, Message: err.Error()})
		return model.Credential{}, fmt.Errorf("凭据验证失败: %w", err)
	}
	encrypted, err := s.box.Seal(secret)
	if err != nil {
		return model.Credential{}, err
	}
	stored.Name = strings.TrimSpace(input.Name)
	stored.Provider = input.Provider
	stored.SecretHint = secretHint(input.Provider, secret)
	stored.EncryptedSecret = encrypted
	stored.UpdatedAt = time.Now()
	if err := s.store.UpdateCredential(ctx, stored); err != nil {
		return model.Credential{}, err
	}
	s.log(ctx, model.AuditLog{Action: "credential.update", Result: "success", CredentialID: stored.ID,
		CredentialName: stored.Name, Provider: stored.Provider, Message: "凭据已验证并更新"})
	if err := s.refreshCredentialWith(ctx, stored, remote); err != nil {
		latest, readErr := s.store.GetCredential(ctx, stored.ID)
		if readErr == nil {
			return latest.Credential, nil
		}
		return stored.Credential, nil
	}
	latest, err := s.store.GetCredential(ctx, credentialID)
	return latest.Credential, err
}

func (s *Service) DeleteCredential(ctx context.Context, credentialID string) error {
	stored, err := s.store.GetCredential(ctx, credentialID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteCredential(ctx, credentialID); err != nil {
		return err
	}
	s.log(ctx, model.AuditLog{Action: "credential.delete", Result: "success", CredentialID: credentialID,
		CredentialName: stored.Name, Provider: stored.Provider, Message: "凭据及其域名和记录缓存已删除"})
	return nil
}

func (s *Service) ListCredentials(ctx context.Context) ([]model.Credential, error) {
	return s.store.ListCredentials(ctx)
}

func (s *Service) providerFor(ctx context.Context, credentialID string) (model.StoredCredential, provider.Provider, error) {
	stored, err := s.store.GetCredential(ctx, credentialID)
	if err != nil {
		return stored, nil, err
	}
	var secret model.CredentialSecret
	if err := s.box.Open(stored.EncryptedSecret, &secret); err != nil {
		return stored, nil, err
	}
	remote, err := s.factory.Build(stored.Provider, secret)
	return stored, remote, err
}

func (s *Service) RefreshCredential(ctx context.Context, credentialID string) error {
	stored, remote, err := s.providerFor(ctx, credentialID)
	if err != nil {
		return err
	}
	return s.refreshCredentialWith(ctx, stored, remote)
}

func (s *Service) refreshCredentialWith(ctx context.Context, stored model.StoredCredential, remote provider.Provider) error {
	domains, err := remote.ListDomains(ctx)
	if err != nil {
		_ = s.store.SetCredentialSync(ctx, stored.ID, model.SyncError, err.Error(), false)
		s.log(ctx, model.AuditLog{Action: "sync.credential", Result: "failed", CredentialID: stored.ID,
			CredentialName: stored.Name, Provider: stored.Provider, Message: err.Error()})
		return err
	}
	if err := s.store.SyncDomains(ctx, stored.ID, domains); err != nil {
		_ = s.store.SetCredentialSync(ctx, stored.ID, model.SyncError, err.Error(), false)
		return err
	}
	if err := s.store.SetCredentialSync(ctx, stored.ID, model.SyncOK, "", true); err != nil {
		return err
	}
	s.log(ctx, model.AuditLog{Action: "sync.credential", Result: "success", CredentialID: stored.ID,
		CredentialName: stored.Name, Provider: stored.Provider, Message: fmt.Sprintf("已同步 %d 个域名", len(domains))})
	return nil
}

type SyncResult struct {
	CredentialID string `json:"credentialId"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

func (s *Service) RefreshAll(ctx context.Context) []SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.store.ListCredentials(ctx)
	if err != nil {
		return []SyncResult{{Success: false, Error: err.Error()}}
	}
	results := make([]SyncResult, 0, len(credentials))
	for _, credential := range credentials {
		err := s.RefreshCredential(ctx, credential.ID)
		result := SyncResult{CredentialID: credential.ID, Success: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (s *Service) ListDomains(ctx context.Context) ([]model.Domain, error) {
	return s.store.ListDomains(ctx)
}

func (s *Service) RefreshDomain(ctx context.Context, domainID string) error {
	domain, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		return err
	}
	stored, remote, err := s.providerFor(ctx, domain.CredentialID)
	if err != nil {
		return err
	}
	records, err := remote.ListRecords(ctx, domain)
	if err != nil {
		_ = s.store.SetDomainSyncError(ctx, domain.ID, err.Error())
		s.log(ctx, model.AuditLog{Action: "sync.domain", Result: "failed", CredentialID: stored.ID,
			CredentialName: stored.Name, Provider: stored.Provider, Domain: domain.Name, Message: err.Error()})
		return err
	}
	if err := s.store.SyncRecords(ctx, domain.ID, records); err != nil {
		return err
	}
	s.log(ctx, model.AuditLog{Action: "sync.domain", Result: "success", CredentialID: stored.ID,
		CredentialName: stored.Name, Provider: stored.Provider, Domain: domain.Name,
		Message: fmt.Sprintf("已同步 %d 条解析记录", len(records))})
	return nil
}

func (s *Service) ListRecords(ctx context.Context, domainID string) (model.Domain, []model.Record, error) {
	domain, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		return domain, nil, err
	}
	records, err := s.store.ListRecords(ctx, domainID)
	return domain, records, err
}

func (s *Service) ensureWritable(domain model.Domain) error {
	if domain.CacheState == model.CacheExpired {
		return ErrExpiredCache
	}
	if domain.LastSyncAt == nil {
		return errors.New("该域名尚未同步解析记录，请先刷新")
	}
	if domain.SyncError != "" {
		return errors.New("最近一次同步失败，请先恢复连接并刷新")
	}
	return nil
}

func sameRecord(cached model.Record, remote model.RemoteRecord) bool {
	normalizeStatus := func(value string) string {
		value = strings.ToLower(value)
		if value == "active" {
			return "enable"
		}
		if value == "disabled" {
			return "disable"
		}
		return value
	}
	return strings.EqualFold(cached.Name, remote.Name) && strings.EqualFold(cached.Type, remote.Type) &&
		cached.Value == remote.Value && cached.TTL == remote.TTL &&
		normalizeStatus(cached.Status) == normalizeStatus(remote.Status) && cached.Line == remote.Line &&
		cached.MX == remote.MX && cached.Weight == remote.Weight && cached.Proxied == remote.Proxied
}

func (s *Service) mutationContext(ctx context.Context, domainID, recordID string) (model.Domain, model.Record, model.StoredCredential, provider.Provider, error) {
	domain, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		return domain, model.Record{}, model.StoredCredential{}, nil, err
	}
	if err := s.ensureWritable(domain); err != nil {
		return domain, model.Record{}, model.StoredCredential{}, nil, err
	}
	record, err := s.store.GetRecord(ctx, domainID, recordID)
	if err != nil {
		return domain, record, model.StoredCredential{}, nil, err
	}
	stored, remote, err := s.providerFor(ctx, domain.CredentialID)
	if err != nil {
		return domain, record, stored, nil, err
	}
	latest, err := remote.GetRecord(ctx, domain, record.RemoteID)
	if err != nil {
		if errors.Is(err, provider.ErrRemoteGone) {
			return domain, record, stored, remote, ErrConflict
		}
		return domain, record, stored, remote, err
	}
	if !sameRecord(record, latest) {
		return domain, record, stored, remote, ErrConflict
	}
	return domain, record, stored, remote, nil
}

func validateRecordInput(input model.RecordInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Type = strings.ToUpper(strings.TrimSpace(input.Type))
	input.Value = strings.TrimSpace(input.Value)
	if input.Name == "" || input.Type == "" || input.Value == "" {
		return errors.New("主机记录、记录类型和记录值不能为空")
	}
	if input.TTL < 0 {
		return errors.New("TTL 不能小于 0")
	}
	return nil
}

func (s *Service) CreateRecord(ctx context.Context, domainID string, input model.RecordInput) (model.RemoteRecord, error) {
	if err := validateRecordInput(input); err != nil {
		return model.RemoteRecord{}, err
	}
	domain, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	if err := s.ensureWritable(domain); err != nil {
		return model.RemoteRecord{}, err
	}
	stored, remote, err := s.providerFor(ctx, domain.CredentialID)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	created, err := remote.CreateRecord(ctx, domain, input)
	if err != nil {
		s.recordLog(ctx, stored, domain, input.Name, input.Type, "record.create", "failed", err.Error())
		return model.RemoteRecord{}, err
	}
	s.recordLog(ctx, stored, domain, input.Name, input.Type, "record.create", "success", "解析记录已新增")
	s.refreshAfterMutation(ctx, domain.ID)
	return created, nil
}

func (s *Service) UpdateRecord(ctx context.Context, domainID, recordID string, input model.RecordInput) (model.RemoteRecord, error) {
	if err := validateRecordInput(input); err != nil {
		return model.RemoteRecord{}, err
	}
	domain, cached, stored, remote, err := s.mutationContext(ctx, domainID, recordID)
	if err != nil {
		if stored.ID != "" {
			s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.update", "failed", err.Error())
		}
		return model.RemoteRecord{}, err
	}
	updated, err := remote.UpdateRecord(ctx, domain, cached.RemoteID, input)
	if err != nil {
		s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.update", "failed", err.Error())
		return model.RemoteRecord{}, err
	}
	s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.update", "success", "解析记录已更新，记录值已脱敏")
	s.refreshAfterMutation(ctx, domain.ID)
	return updated, nil
}

func (s *Service) DeleteRecord(ctx context.Context, domainID, recordID string) error {
	domain, cached, stored, remote, err := s.mutationContext(ctx, domainID, recordID)
	if err != nil {
		if stored.ID != "" {
			s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.delete", "failed", err.Error())
		}
		return err
	}
	if err := remote.DeleteRecord(ctx, domain, cached.RemoteID); err != nil {
		s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.delete", "failed", err.Error())
		return err
	}
	s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.delete", "success", "解析记录已删除")
	s.refreshAfterMutation(ctx, domain.ID)
	return nil
}

func (s *Service) SetRecordStatus(ctx context.Context, domainID, recordID string, enabled bool) error {
	domain, cached, stored, remote, err := s.mutationContext(ctx, domainID, recordID)
	if err != nil {
		if stored.ID != "" {
			s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.status", "failed", err.Error())
		}
		return err
	}
	if !cached.SupportsDisable {
		return provider.ErrUnsupported
	}
	if err := remote.SetRecordStatus(ctx, domain, cached.RemoteID, enabled); err != nil {
		s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.status", "failed", err.Error())
		return err
	}
	message := "解析记录已暂停"
	if enabled {
		message = "解析记录已启用"
	}
	s.recordLog(ctx, stored, domain, cached.Name, cached.Type, "record.status", "success", message)
	s.refreshAfterMutation(ctx, domain.ID)
	return nil
}

type BatchItemResult struct {
	RecordID string `json:"recordId"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

func (s *Service) BatchDelete(ctx context.Context, domainID string, recordIDs []string) []BatchItemResult {
	results := make([]BatchItemResult, 0, len(recordIDs))
	for _, recordID := range unique(recordIDs) {
		err := s.DeleteRecord(ctx, domainID, recordID)
		result := BatchItemResult{RecordID: recordID, Success: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (s *Service) BatchStatus(ctx context.Context, domainID string, recordIDs []string, enabled bool) []BatchItemResult {
	results := make([]BatchItemResult, 0, len(recordIDs))
	for _, recordID := range unique(recordIDs) {
		err := s.SetRecordStatus(ctx, domainID, recordID, enabled)
		result := BatchItemResult{RecordID: recordID, Success: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (s *Service) refreshAfterMutation(ctx context.Context, domainID string) {
	if err := s.RefreshDomain(ctx, domainID); err != nil {
		_ = s.store.SetDomainSyncError(ctx, domainID, "远端操作成功，但刷新缓存失败: "+err.Error())
	}
}

func (s *Service) recordLog(ctx context.Context, stored model.StoredCredential, domain model.Domain, name, recordType, action, result, message string) {
	s.log(ctx, model.AuditLog{CredentialID: stored.ID, CredentialName: stored.Name, Provider: stored.Provider,
		Domain: domain.Name, RecordName: name, RecordType: recordType, Action: action, Result: result, Message: message})
}

func (s *Service) log(ctx context.Context, entry model.AuditLog) {
	entry.Operator = s.operator
	_ = s.store.AppendLog(ctx, entry)
}

func (s *Service) Capability(kind string) (model.Capability, error) {
	return provider.Capability(kind)
}
