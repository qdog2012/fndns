package provider

import (
	"bytes"
	"context"
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

type Cloudflare struct {
	client  *http.Client
	token   string
	baseURL string
}

func NewCloudflare(client *http.Client, token string) *Cloudflare {
	return &Cloudflare{client: client, token: token, baseURL: "https://api.cloudflare.com/client/v4"}
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfEnvelope struct {
	Success    bool            `json:"success"`
	Errors     []cfError       `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
		Count      int `json:"count"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

type cfZone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Plan   struct {
		Name string `json:"name"`
	} `json:"plan"`
}

type cfDNSRecord struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Content    string         `json:"content"`
	TTL        int            `json:"ttl"`
	Proxied    bool           `json:"proxied"`
	Proxiable  bool           `json:"proxiable"`
	Priority   int            `json:"priority"`
	Comment    string         `json:"comment"`
	ModifiedOn string         `json:"modified_on"`
	Data       map[string]any `json:"data"`
}

func (c *Cloudflare) do(ctx context.Context, method, path string, body any, target any) (cfEnvelope, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return cfEnvelope{}, err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.baseURL, "/")+path, payload)
	if err != nil {
		return cfEnvelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return cfEnvelope{}, fmt.Errorf("连接 Cloudflare 失败: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 4<<20)
	var envelope cfEnvelope
	if err := json.NewDecoder(limited).Decode(&envelope); err != nil {
		return envelope, fmt.Errorf("解析 Cloudflare 响应失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	if !envelope.Success || resp.StatusCode >= 400 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, item := range envelope.Errors {
			messages = append(messages, item.Message)
			if item.Code == 81044 {
				return envelope, ErrRemoteGone
			}
		}
		if len(messages) == 0 {
			messages = append(messages, resp.Status)
		}
		return envelope, fmt.Errorf("Cloudflare API: %s", strings.Join(messages, "; "))
	}
	if target != nil && len(envelope.Result) > 0 && string(envelope.Result) != "null" {
		if err := json.Unmarshal(envelope.Result, target); err != nil {
			return envelope, fmt.Errorf("解析 Cloudflare 数据失败: %w", err)
		}
	}
	return envelope, nil
}

func (c *Cloudflare) Validate(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/user/tokens/verify", nil, nil)
	return err
}

func (c *Cloudflare) ListDomains(ctx context.Context) ([]model.RemoteDomain, error) {
	items := make([]model.RemoteDomain, 0)
	for page := 1; ; page++ {
		var zones []cfZone
		envelope, err := c.do(ctx, http.MethodGet, "/zones?per_page=50&page="+strconv.Itoa(page)+"&order=name&direction=asc", nil, &zones)
		if err != nil {
			return nil, err
		}
		for _, zone := range zones {
			items = append(items, model.RemoteDomain{RemoteID: zone.ID, Name: zone.Name, Status: zone.Status, Grade: zone.Plan.Name})
		}
		if len(zones) == 0 || envelope.ResultInfo.TotalPages == 0 || page >= envelope.ResultInfo.TotalPages {
			break
		}
	}
	return items, nil
}

func (c *Cloudflare) ListRecords(ctx context.Context, domain model.Domain) ([]model.RemoteRecord, error) {
	items := make([]model.RemoteRecord, 0)
	base := "/zones/" + url.PathEscape(domain.RemoteID) + "/dns_records"
	for page := 1; ; page++ {
		var records []cfDNSRecord
		envelope, err := c.do(ctx, http.MethodGet, base+"?per_page=100&page="+strconv.Itoa(page)+"&order=name&direction=asc", nil, &records)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			items = append(items, fromCFRecord(domain.Name, record))
		}
		if len(records) == 0 || envelope.ResultInfo.TotalPages == 0 || page >= envelope.ResultInfo.TotalPages {
			break
		}
	}
	return items, nil
}

func fromCFRecord(zone string, record cfDNSRecord) model.RemoteRecord {
	name := relativeName(record.Name, zone)
	value := record.Content
	priority, weight := record.Priority, 0
	if record.Type == "SRV" && record.Data != nil {
		priority = intFromAny(record.Data["priority"])
		weight = intFromAny(record.Data["weight"])
		value = strings.TrimSpace(fmt.Sprintf("%d %v", intFromAny(record.Data["port"]), record.Data["target"]))
	}
	if record.Type == "CAA" && record.Data != nil {
		value = strings.TrimSpace(fmt.Sprintf("%d %v %v", intFromAny(record.Data["flags"]), record.Data["tag"], record.Data["value"]))
	}
	var modified *time.Time
	if record.ModifiedOn != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, record.ModifiedOn); err == nil {
			modified = &parsed
		}
	}
	return model.RemoteRecord{
		RemoteID: record.ID, Name: name, Type: record.Type, Value: value, TTL: record.TTL,
		Status: "active", MX: priority, Weight: weight, Proxied: record.Proxied,
		SupportsProxied: record.Proxiable, SupportsDisable: false, Remark: record.Comment,
		UpdatedAt: modified,
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		v, _ := typed.Int64()
		return int(v)
	case string:
		v, _ := strconv.Atoi(typed)
		return v
	default:
		return 0
	}
}

func relativeName(name, zone string) string {
	name = strings.TrimSuffix(name, ".")
	zone = strings.TrimSuffix(zone, ".")
	if strings.EqualFold(name, zone) {
		return "@"
	}
	suffix := "." + strings.ToLower(zone)
	if strings.HasSuffix(strings.ToLower(name), suffix) {
		return name[:len(name)-len(suffix)]
	}
	return name
}

func absoluteName(name, zone string) string {
	name = strings.TrimSpace(strings.TrimSuffix(name, "."))
	if name == "" || name == "@" {
		return zone
	}
	if strings.EqualFold(name, zone) || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(zone)) {
		return name
	}
	return name + "." + zone
}

func (c *Cloudflare) GetRecord(ctx context.Context, domain model.Domain, remoteID string) (model.RemoteRecord, error) {
	var record cfDNSRecord
	_, err := c.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(domain.RemoteID)+"/dns_records/"+url.PathEscape(remoteID), nil, &record)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	return fromCFRecord(domain.Name, record), nil
}

func cfRecordBody(domain model.Domain, input model.RecordInput) (map[string]any, error) {
	typeName := strings.ToUpper(strings.TrimSpace(input.Type))
	if input.TTL < 0 {
		return nil, errors.New("TTL 不能小于 0")
	}
	body := map[string]any{
		"type": typeName,
		"name": absoluteName(input.Name, domain.Name),
		"ttl":  input.TTL,
	}
	if body["ttl"] == 0 {
		body["ttl"] = 1
	}
	switch typeName {
	case "MX":
		body["content"] = strings.TrimSpace(input.Value)
		body["priority"] = input.MX
	case "SRV":
		parts := strings.Fields(input.Value)
		if len(parts) != 2 {
			return nil, errors.New("SRV 记录值格式应为“端口 目标”，例如“443 service.example.com”")
		}
		port, err := strconv.Atoi(parts[0])
		if err != nil || port < 0 || port > 65535 {
			return nil, errors.New("SRV 端口无效")
		}
		body["data"] = map[string]any{"priority": input.MX, "weight": input.Weight, "port": port, "target": parts[1]}
	case "CAA":
		parts := strings.Fields(input.Value)
		if len(parts) < 3 {
			return nil, errors.New("CAA 记录值格式应为“标志 标签 值”，例如“0 issue letsencrypt.org”")
		}
		flags, err := strconv.Atoi(parts[0])
		if err != nil || flags < 0 || flags > 255 {
			return nil, errors.New("CAA 标志无效")
		}
		body["data"] = map[string]any{"flags": flags, "tag": parts[1], "value": strings.Join(parts[2:], " ")}
	default:
		body["content"] = strings.TrimSpace(input.Value)
	}
	if input.Remark != "" {
		body["comment"] = input.Remark
	}
	if supportsCloudflareProxy(typeName) {
		body["proxied"] = input.Proxied
	}
	return body, nil
}

func supportsCloudflareProxy(recordType string) bool {
	switch recordType {
	case "A", "AAAA", "CNAME":
		return true
	default:
		return false
	}
}

func (c *Cloudflare) CreateRecord(ctx context.Context, domain model.Domain, input model.RecordInput) (model.RemoteRecord, error) {
	body, err := cfRecordBody(domain, input)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	var record cfDNSRecord
	_, err = c.do(ctx, http.MethodPost, "/zones/"+url.PathEscape(domain.RemoteID)+"/dns_records", body, &record)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	return fromCFRecord(domain.Name, record), nil
}

func (c *Cloudflare) UpdateRecord(ctx context.Context, domain model.Domain, remoteID string, input model.RecordInput) (model.RemoteRecord, error) {
	body, err := cfRecordBody(domain, input)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	var record cfDNSRecord
	_, err = c.do(ctx, http.MethodPut, "/zones/"+url.PathEscape(domain.RemoteID)+"/dns_records/"+url.PathEscape(remoteID), body, &record)
	if err != nil {
		return model.RemoteRecord{}, err
	}
	return fromCFRecord(domain.Name, record), nil
}

func (c *Cloudflare) DeleteRecord(ctx context.Context, domain model.Domain, remoteID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/zones/"+url.PathEscape(domain.RemoteID)+"/dns_records/"+url.PathEscape(remoteID), nil, nil)
	return err
}

func (c *Cloudflare) SetRecordStatus(context.Context, model.Domain, string, bool) error {
	return ErrUnsupported
}

func cloudflareCapability() model.Capability {
	return model.Capability{
		Provider:        model.ProviderCloudflare,
		RecordTypes:     []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "CERT", "DNSKEY", "DS", "HTTPS", "LOC", "NAPTR", "PTR", "SMIMEA", "SSHFP", "SVCB", "TLSA", "URI"},
		SupportsProxied: true, SupportsDisable: false, TTLAutomatic: true, DefaultTTL: 1,
	}
}

func (c *Cloudflare) Capability() model.Capability { return cloudflareCapability() }
