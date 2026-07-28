package model

import "time"

const (
	ProviderCloudflare = "cloudflare"
	ProviderTencent    = "tencent"
	SyncNever          = "never"
	SyncOK             = "ok"
	SyncError          = "error"
	CacheFresh         = "fresh"
	CacheExpired       = "expired"
	CacheNever         = "never"
)

type Credential struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Provider       string     `json:"provider"`
	SecretHint     string     `json:"secretHint"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastSyncAt     *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncStatus string     `json:"lastSyncStatus"`
	LastSyncError  string     `json:"lastSyncError,omitempty"`
	DomainCount    int        `json:"domainCount"`
}

type CredentialSecret struct {
	Token     string `json:"token,omitempty"`
	SecretID  string `json:"secretId,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

type CredentialInput struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Token     string `json:"token,omitempty"`
	SecretID  string `json:"secretId,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

func (in CredentialInput) Secret() CredentialSecret {
	return CredentialSecret{Token: in.Token, SecretID: in.SecretID, SecretKey: in.SecretKey}
}

type StoredCredential struct {
	Credential
	EncryptedSecret []byte `json:"-"`
}

type Domain struct {
	ID             string     `json:"id"`
	CredentialID   string     `json:"credentialId"`
	CredentialName string     `json:"credentialName"`
	Provider       string     `json:"provider"`
	RemoteID       string     `json:"remoteId"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Grade          string     `json:"grade,omitempty"`
	RecordCount    int        `json:"recordCount"`
	LastSyncAt     *time.Time `json:"lastSyncAt,omitempty"`
	SyncError      string     `json:"syncError,omitempty"`
	CacheState     string     `json:"cacheState"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Record struct {
	ID              string     `json:"id"`
	DomainID        string     `json:"domainId"`
	RemoteID        string     `json:"remoteId"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`
	Value           string     `json:"value"`
	TTL             int        `json:"ttl"`
	Status          string     `json:"status"`
	Line            string     `json:"line,omitempty"`
	LineID          string     `json:"lineId,omitempty"`
	MX              int        `json:"mx,omitempty"`
	Weight          int        `json:"weight,omitempty"`
	Proxied         bool       `json:"proxied"`
	SupportsProxied bool       `json:"supportsProxied"`
	SupportsDisable bool       `json:"supportsDisable"`
	Remark          string     `json:"remark,omitempty"`
	RemoteUpdatedAt *time.Time `json:"remoteUpdatedAt,omitempty"`
	LastSyncAt      time.Time  `json:"lastSyncAt"`
}

type RecordInput struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	TTL     int    `json:"ttl"`
	Status  string `json:"status,omitempty"`
	Line    string `json:"line,omitempty"`
	LineID  string `json:"lineId,omitempty"`
	MX      int    `json:"mx,omitempty"`
	Weight  int    `json:"weight,omitempty"`
	Proxied bool   `json:"proxied"`
	Remark  string `json:"remark,omitempty"`
}

type RemoteDomain struct {
	RemoteID    string
	Name        string
	Status      string
	Grade       string
	RecordCount int
}

type RemoteRecord struct {
	RemoteID        string
	Name            string
	Type            string
	Value           string
	TTL             int
	Status          string
	Line            string
	LineID          string
	MX              int
	Weight          int
	Proxied         bool
	SupportsProxied bool
	SupportsDisable bool
	Remark          string
	UpdatedAt       *time.Time
}

type Capability struct {
	Provider        string   `json:"provider"`
	RecordTypes     []string `json:"recordTypes"`
	SupportsProxied bool     `json:"supportsProxied"`
	SupportsDisable bool     `json:"supportsDisable"`
	SupportsLine    bool     `json:"supportsLine"`
	SupportsWeight  bool     `json:"supportsWeight"`
	TTLAutomatic    bool     `json:"ttlAutomatic"`
	DefaultTTL      int      `json:"defaultTtl"`
	Lines           []string `json:"lines,omitempty"`
}

type AuditLog struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"createdAt"`
	Operator       string    `json:"operator"`
	CredentialID   string    `json:"credentialId,omitempty"`
	CredentialName string    `json:"credentialName,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Domain         string    `json:"domain,omitempty"`
	RecordName     string    `json:"recordName,omitempty"`
	RecordType     string    `json:"recordType,omitempty"`
	Action         string    `json:"action"`
	Result         string    `json:"result"`
	Message        string    `json:"message,omitempty"`
}

type LogFilter struct {
	From         *time.Time
	To           *time.Time
	CredentialID string
	Domain       string
	Action       string
	Result       string
	Limit        int
	Offset       int
}

type Overview struct {
	DomainCount        int        `json:"domainCount"`
	RecordCount        int        `json:"recordCount"`
	CredentialCount    int        `json:"credentialCount"`
	CredentialFailures int        `json:"credentialFailures"`
	LastSyncAt         *time.Time `json:"lastSyncAt,omitempty"`
}
