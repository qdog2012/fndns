package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fndns/manager/internal/model"
)

const tencentVersion = "2021-03-23"

type Tencent struct {
	client    *http.Client
	secretID  string
	secretKey string
	endpoint  string
	now       func() time.Time
}

func NewTencent(client *http.Client, secretID, secretKey string) *Tencent {
	return &Tencent{client: client, secretID: secretID, secretKey: secretKey,
		endpoint: "https://dnspod.tencentcloudapi.com", now: time.Now}
}

type tcError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type tcEnvelope struct {
	Response json.RawMessage `json:"Response"`
}

type tcResponseMeta struct {
	Error     *tcError `json:"Error"`
	RequestID string   `json:"RequestId"`
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func (c *Tencent) authorization(action string, payload []byte, at time.Time, host string) string {
	timestampValue := strconv.FormatInt(at.Unix(), 10)
	date := at.UTC().Format("2006-01-02")
	canonicalHeaders := "content-type:application/json; charset=utf-8\n" + "host:" + host + "\n" + "x-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256Hex(payload)
	credentialScope := date + "/dnspod/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestampValue + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+c.secretKey), date)
	secretService := hmacSHA256(secretDate, "dnspod")
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return "TC3-HMAC-SHA256 Credential=" + c.secretID + "/" + credentialScope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func (c *Tencent) call(ctx context.Context, action string, input any, target any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}
	at := c.now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authorization(action, payload, at, parsed.Host))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", parsed.Host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(at.Unix(), 10))
	req.Header.Set("X-TC-Version", tencentVersion)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("连接腾讯云 DNSPod 失败: %w", err)
	}
	defer resp.Body.Close()
	var envelope tcEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("解析腾讯云响应失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	var meta tcResponseMeta
	if err := json.Unmarshal(envelope.Response, &meta); err != nil {
		return fmt.Errorf("解析腾讯云响应元数据失败: %w", err)
	}
	if meta.Error != nil {
		if strings.Contains(meta.Error.Code, "RecordIdInvalid") || strings.Contains(meta.Error.Code, "RecordNotExist") {
			return ErrRemoteGone
		}
		return fmt.Errorf("腾讯云 DNSPod API: %s (%s)", meta.Error.Message, meta.Error.Code)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("腾讯云 DNSPod API: HTTP %d", resp.StatusCode)
	}
	if target != nil {
		if err := json.Unmarshal(envelope.Response, target); err != nil {
			return fmt.Errorf("解析腾讯云数据失败: %w", err)
		}
	}
	return nil
}

type tcDomain struct {
	DomainID    uint64 `json:"DomainId"`
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	Grade       string `json:"Grade"`
	RecordCount uint64 `json:"RecordCount"`
}

type tcDomainListResponse struct {
	DomainList      []tcDomain `json:"DomainList"`
	DomainCountInfo struct {
		AllTotal uint64 `json:"AllTotal"`
	} `json:"DomainCountInfo"`
}

func (c *Tencent) Validate(ctx context.Context) error {
	var response tcDomainListResponse
	return c.call(ctx, "DescribeDomainList", map[string]any{"Offset": 0, "Limit": 1}, &response)
}

func (c *Tencent) ListDomains(ctx context.Context) ([]model.RemoteDomain, error) {
	items := make([]model.RemoteDomain, 0)
	for offset := 0; ; offset += 100 {
		var response tcDomainListResponse
		if err := c.call(ctx, "DescribeDomainList", map[string]any{"Offset": offset, "Limit": 100}, &response); err != nil {
			return nil, err
		}
		for _, domain := range response.DomainList {
			items = append(items, model.RemoteDomain{RemoteID: strconv.FormatUint(domain.DomainID, 10), Name: domain.Name,
				Status: strings.ToLower(domain.Status), Grade: domain.Grade, RecordCount: int(domain.RecordCount)})
		}
		if len(response.DomainList) < 100 || uint64(len(items)) >= response.DomainCountInfo.AllTotal {
			break
		}
	}
	return items, nil
}

type tcRecord struct {
	RecordID  uint64  `json:"RecordId"`
	Value     string  `json:"Value"`
	Status    string  `json:"Status"`
	UpdatedOn string  `json:"UpdatedOn"`
	Name      string  `json:"Name"`
	Line      string  `json:"Line"`
	LineID    string  `json:"LineId"`
	Type      string  `json:"Type"`
	Weight    *uint64 `json:"Weight"`
	Remark    string  `json:"Remark"`
	TTL       uint64  `json:"TTL"`
	MX        uint64  `json:"MX"`
}

type tcRecordListResponse struct {
	RecordList      []tcRecord `json:"RecordList"`
	RecordCountInfo struct {
		TotalCount uint64 `json:"TotalCount"`
	} `json:"RecordCountInfo"`
}

func fromTCRecord(record tcRecord) model.RemoteRecord {
	var updatedAt *time.Time
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, time.RFC3339Nano} {
		if parsed, err := time.ParseInLocation(layout, record.UpdatedOn, time.Local); err == nil {
			updatedAt = &parsed
			break
		}
	}
	weight := 0
	if record.Weight != nil {
		weight = int(*record.Weight)
	}
	value := record.Value
	if strings.EqualFold(record.Type, "SRV") {
		parts := strings.Fields(record.Value)
		if len(parts) >= 3 {
			if parsedWeight, err := strconv.Atoi(parts[0]); err == nil {
				weight = parsedWeight
				value = strings.Join(parts[1:], " ")
			}
		}
	}
	return model.RemoteRecord{RemoteID: strconv.FormatUint(record.RecordID, 10), Name: record.Name,
		Type: record.Type, Value: value, TTL: int(record.TTL), Status: strings.ToLower(record.Status),
		Line: record.Line, LineID: record.LineID, MX: int(record.MX), Weight: weight,
		SupportsDisable: true, Remark: record.Remark, UpdatedAt: updatedAt}
}

func (c *Tencent) ListRecords(ctx context.Context, domain model.Domain) ([]model.RemoteRecord, error) {
	items := make([]model.RemoteRecord, 0)
	for offset := 0; ; offset += 100 {
		var response tcRecordListResponse
		input := map[string]any{"Domain": domain.Name, "Offset": offset, "Limit": 100}
		if err := c.call(ctx, "DescribeRecordList", input, &response); err != nil {
			return nil, err
		}
		for _, record := range response.RecordList {
			items = append(items, fromTCRecord(record))
		}
		if len(response.RecordList) < 100 || uint64(len(items)) >= response.RecordCountInfo.TotalCount {
			break
		}
	}
	return items, nil
}

func (c *Tencent) GetRecord(ctx context.Context, domain model.Domain, remoteID string) (model.RemoteRecord, error) {
	recordID, err := strconv.ParseUint(remoteID, 10, 64)
	if err != nil {
		return model.RemoteRecord{}, errors.New("腾讯云记录 ID 无效")
	}
	var response struct {
		RecordInfo tcRecord `json:"RecordInfo"`
	}
	if err := c.call(ctx, "DescribeRecord", map[string]any{"Domain": domain.Name, "RecordId": recordID}, &response); err != nil {
		return model.RemoteRecord{}, err
	}
	response.RecordInfo.RecordID = recordID
	return fromTCRecord(response.RecordInfo), nil
}

func tcRecordInput(domain model.Domain, input model.RecordInput) map[string]any {
	line := strings.TrimSpace(input.Line)
	if line == "" {
		line = "默认"
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 600
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "@"
	}
	value := map[string]any{"Domain": domain.Name, "SubDomain": name, "RecordType": strings.ToUpper(input.Type),
		"RecordLine": line, "Value": strings.TrimSpace(input.Value), "TTL": ttl}
	if strings.EqualFold(input.Type, "SRV") {
		value["Value"] = fmt.Sprintf("%d %s", input.Weight, strings.TrimSpace(input.Value))
	}
	if input.MX > 0 || strings.EqualFold(input.Type, "MX") || strings.EqualFold(input.Type, "SRV") {
		value["MX"] = input.MX
	}
	if input.Weight > 0 && !strings.EqualFold(input.Type, "SRV") {
		value["Weight"] = input.Weight
	}
	if input.Status != "" {
		status := strings.ToUpper(input.Status)
		if status == "ACTIVE" {
			status = "ENABLE"
		}
		value["Status"] = status
	}
	return value
}

func (c *Tencent) CreateRecord(ctx context.Context, domain model.Domain, input model.RecordInput) (model.RemoteRecord, error) {
	var response struct {
		RecordID uint64 `json:"RecordId"`
	}
	if err := c.call(ctx, "CreateRecord", tcRecordInput(domain, input), &response); err != nil {
		return model.RemoteRecord{}, err
	}
	return c.GetRecord(ctx, domain, strconv.FormatUint(response.RecordID, 10))
}

func (c *Tencent) UpdateRecord(ctx context.Context, domain model.Domain, remoteID string, input model.RecordInput) (model.RemoteRecord, error) {
	recordID, err := strconv.ParseUint(remoteID, 10, 64)
	if err != nil {
		return model.RemoteRecord{}, errors.New("腾讯云记录 ID 无效")
	}
	payload := tcRecordInput(domain, input)
	payload["RecordId"] = recordID
	if err := c.call(ctx, "ModifyRecord", payload, &struct{}{}); err != nil {
		return model.RemoteRecord{}, err
	}
	return c.GetRecord(ctx, domain, remoteID)
}

func (c *Tencent) DeleteRecord(ctx context.Context, domain model.Domain, remoteID string) error {
	recordID, err := strconv.ParseUint(remoteID, 10, 64)
	if err != nil {
		return errors.New("腾讯云记录 ID 无效")
	}
	return c.call(ctx, "DeleteRecord", map[string]any{"Domain": domain.Name, "RecordId": recordID}, &struct{}{})
}

func (c *Tencent) SetRecordStatus(ctx context.Context, domain model.Domain, remoteID string, enabled bool) error {
	recordID, err := strconv.ParseUint(remoteID, 10, 64)
	if err != nil {
		return errors.New("腾讯云记录 ID 无效")
	}
	status := "DISABLE"
	if enabled {
		status = "ENABLE"
	}
	return c.call(ctx, "ModifyRecordStatus", map[string]any{"Domain": domain.Name, "RecordId": recordID, "Status": status}, &struct{}{})
}

func tencentCapability() model.Capability {
	return model.Capability{
		Provider:        model.ProviderTencent,
		RecordTypes:     []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "HTTPS", "SVCB"},
		SupportsDisable: true, SupportsLine: true, SupportsWeight: true, DefaultTTL: 600,
		Lines: []string{"默认", "境内", "境外", "电信", "联通", "移动", "教育网", "搜索引擎"},
	}
}

func (c *Tencent) Capability() model.Capability { return tencentCapability() }
