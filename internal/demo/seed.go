package demo

import (
	"context"
	"time"

	"github.com/fndns/manager/internal/model"
	"github.com/fndns/manager/internal/secretbox"
	"github.com/fndns/manager/internal/store"
)

func Seed(ctx context.Context, storage *store.Store, box *secretbox.Box) error {
	existing, err := storage.ListCredentials(ctx)
	if err != nil || len(existing) > 0 {
		return err
	}
	now := time.Now()
	credentials := []struct {
		item   model.StoredCredential
		secret model.CredentialSecret
	}{
		{item: model.StoredCredential{Credential: model.Credential{ID: "demo-cloudflare", Name: "产品环境 Cloudflare", Provider: model.ProviderCloudflare, SecretHint: "demo••••oken", CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour)}}, secret: model.CredentialSecret{Token: "demo-token"}},
		{item: model.StoredCredential{Credential: model.Credential{ID: "demo-tencent", Name: "家庭腾讯云", Provider: model.ProviderTencent, SecretHint: "AKID••••DEMO", CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)}}, secret: model.CredentialSecret{SecretID: "AKID-DEMO", SecretKey: "demo-secret"}},
	}
	for index := range credentials {
		encrypted, err := box.Seal(credentials[index].secret)
		if err != nil {
			return err
		}
		credentials[index].item.EncryptedSecret = encrypted
		if err := storage.CreateCredential(ctx, credentials[index].item); err != nil {
			return err
		}
		_ = storage.SetCredentialSync(ctx, credentials[index].item.ID, model.SyncOK, "", true)
	}
	if err := storage.SyncDomains(ctx, "demo-cloudflare", []model.RemoteDomain{
		{RemoteID: "cf-zone-1", Name: "northstar.dev", Status: "active", Grade: "Free", RecordCount: 5},
		{RemoteID: "cf-zone-2", Name: "edge-console.io", Status: "active", Grade: "Pro", RecordCount: 3},
		{RemoteID: "cf-zone-3", Name: "shared-example.com", Status: "active", Grade: "Free", RecordCount: 2},
	}); err != nil {
		return err
	}
	if err := storage.SyncDomains(ctx, "demo-tencent", []model.RemoteDomain{
		{RemoteID: "101", Name: "yunjian.cn", Status: "enable", Grade: "DP_FREE", RecordCount: 4},
		{RemoteID: "102", Name: "home-lab.cn", Status: "enable", Grade: "DP_PLUS", RecordCount: 3},
		{RemoteID: "103", Name: "shared-example.com", Status: "enable", Grade: "DP_FREE", RecordCount: 8},
	}); err != nil {
		return err
	}
	domains, err := storage.ListDomains(ctx)
	if err != nil {
		return err
	}
	for _, domain := range domains {
		records := cloudflareRecords()
		if domain.Provider == model.ProviderTencent {
			records = tencentRecords()
		}
		if err := storage.SyncRecords(ctx, domain.ID, records); err != nil {
			return err
		}
	}
	return nil
}

func cloudflareRecords() []model.RemoteRecord {
	return []model.RemoteRecord{
		{RemoteID: "r1", Name: "@", Type: "A", Value: "203.0.113.18", TTL: 1, Status: "active", Proxied: true, SupportsProxied: true},
		{RemoteID: "r2", Name: "www", Type: "CNAME", Value: "northstar.dev", TTL: 1, Status: "active", Proxied: true, SupportsProxied: true},
		{RemoteID: "r3", Name: "api", Type: "AAAA", Value: "2001:db8::18", TTL: 300, Status: "active", SupportsProxied: true},
		{RemoteID: "r4", Name: "@", Type: "MX", Value: "mail.example.net", TTL: 3600, Status: "active", MX: 10},
		{RemoteID: "r5", Name: "_verify", Type: "TXT", Value: "demo-verification-value", TTL: 300, Status: "active", Remark: "站点验证"},
	}
}

func tencentRecords() []model.RemoteRecord {
	return []model.RemoteRecord{
		{RemoteID: "201", Name: "@", Type: "A", Value: "198.51.100.20", TTL: 600, Status: "enable", Line: "默认", SupportsDisable: true},
		{RemoteID: "202", Name: "www", Type: "CNAME", Value: "cdn.example.cn", TTL: 600, Status: "enable", Line: "境内", SupportsDisable: true},
		{RemoteID: "203", Name: "backup", Type: "A", Value: "198.51.100.21", TTL: 600, Status: "disable", Line: "电信", SupportsDisable: true},
		{RemoteID: "204", Name: "@", Type: "TXT", Value: "v=spf1 include:spf.example.cn -all", TTL: 600, Status: "enable", Line: "默认", SupportsDisable: true},
	}
}
