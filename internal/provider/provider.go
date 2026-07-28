package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fndns/manager/internal/model"
)

var (
	ErrUnsupported = errors.New("当前平台不支持此操作")
	ErrRemoteGone  = errors.New("远端记录已不存在")
)

type Provider interface {
	Validate(context.Context) error
	ListDomains(context.Context) ([]model.RemoteDomain, error)
	ListRecords(context.Context, model.Domain) ([]model.RemoteRecord, error)
	GetRecord(context.Context, model.Domain, string) (model.RemoteRecord, error)
	CreateRecord(context.Context, model.Domain, model.RecordInput) (model.RemoteRecord, error)
	UpdateRecord(context.Context, model.Domain, string, model.RecordInput) (model.RemoteRecord, error)
	DeleteRecord(context.Context, model.Domain, string) error
	SetRecordStatus(context.Context, model.Domain, string, bool) error
	Capability() model.Capability
}

type Factory struct {
	Client          *http.Client
	CloudflareURL   string
	TencentEndpoint string
}

func NewFactory(client *http.Client) *Factory {
	if client == nil {
		client = &http.Client{}
	}
	return &Factory{Client: client}
}

func (f *Factory) Build(kind string, secret model.CredentialSecret) (Provider, error) {
	switch kind {
	case model.ProviderCloudflare:
		if secret.Token == "" {
			return nil, errors.New("Cloudflare API Token 不能为空")
		}
		client := NewCloudflare(f.Client, secret.Token)
		if f.CloudflareURL != "" {
			client.baseURL = f.CloudflareURL
		}
		return client, nil
	case model.ProviderTencent:
		if secret.SecretID == "" || secret.SecretKey == "" {
			return nil, errors.New("腾讯云 SecretId 和 SecretKey 不能为空")
		}
		client := NewTencent(f.Client, secret.SecretID, secret.SecretKey)
		if f.TencentEndpoint != "" {
			client.endpoint = f.TencentEndpoint
		}
		return client, nil
	default:
		return nil, fmt.Errorf("不支持的平台: %s", kind)
	}
}

func Capability(kind string) (model.Capability, error) {
	switch kind {
	case model.ProviderCloudflare:
		return cloudflareCapability(), nil
	case model.ProviderTencent:
		return tencentCapability(), nil
	default:
		return model.Capability{}, fmt.Errorf("不支持的平台: %s", kind)
	}
}
