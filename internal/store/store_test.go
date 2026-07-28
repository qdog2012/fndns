package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fndns/manager/internal/model"
)

func TestDomainsAreDeduplicatedByCredentialCreationOrder(t *testing.T) {
	ctx := context.Background()
	storage, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, credential := range []model.StoredCredential{
		{Credential: model.Credential{ID: "first", Name: "第一组", Provider: model.ProviderCloudflare, CreatedAt: created, UpdatedAt: created}, EncryptedSecret: []byte("a")},
		{Credential: model.Credential{ID: "second", Name: "第二组", Provider: model.ProviderTencent, CreatedAt: created.Add(time.Minute), UpdatedAt: created.Add(time.Minute)}, EncryptedSecret: []byte("b")},
	} {
		if err := storage.CreateCredential(ctx, credential); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.SyncDomains(ctx, "first", []model.RemoteDomain{{RemoteID: "z1", Name: "Example.COM"}}); err != nil {
		t.Fatal(err)
	}
	if err := storage.SyncDomains(ctx, "second", []model.RemoteDomain{{RemoteID: "99", Name: "example.com"}, {RemoteID: "100", Name: "other.cn"}}); err != nil {
		t.Fatal(err)
	}
	domains, err := storage.ListDomains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 deduplicated domains, got %d", len(domains))
	}
	var example model.Domain
	for _, domain := range domains {
		if domain.Name == "example.com" {
			example = domain
		}
	}
	if example.CredentialID != "first" {
		t.Fatalf("expected oldest credential to win, got %q", example.CredentialID)
	}
}

func TestCacheExpiresAfterSixMonthsButDataRemains(t *testing.T) {
	ctx := context.Background()
	storage, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	storage.now = func() time.Time { return now.AddDate(0, -7, 0) }
	credential := model.StoredCredential{Credential: model.Credential{ID: "c1", Name: "测试", Provider: model.ProviderCloudflare, CreatedAt: storage.now(), UpdatedAt: storage.now()}, EncryptedSecret: []byte("a")}
	if err := storage.CreateCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	if err := storage.SyncDomains(ctx, "c1", []model.RemoteDomain{{RemoteID: "z1", Name: "example.com"}}); err != nil {
		t.Fatal(err)
	}
	domains, _ := storage.ListDomains(ctx)
	if err := storage.SyncRecords(ctx, domains[0].ID, []model.RemoteRecord{{RemoteID: "r1", Name: "@", Type: "A", Value: "192.0.2.1", TTL: 300}}); err != nil {
		t.Fatal(err)
	}
	storage.now = func() time.Time { return now }
	domain, err := storage.GetDomain(ctx, domains[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if domain.CacheState != model.CacheExpired {
		t.Fatalf("expected expired cache, got %q", domain.CacheState)
	}
	records, err := storage.ListRecords(ctx, domain.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("expired records should remain readable: records=%d err=%v", len(records), err)
	}
}

func TestSyncRecordsPreservesLocalIDs(t *testing.T) {
	ctx := context.Background()
	storage, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now()
	credential := model.StoredCredential{Credential: model.Credential{ID: "c1", Name: "测试", Provider: model.ProviderTencent, CreatedAt: now, UpdatedAt: now}, EncryptedSecret: []byte("a")}
	if err := storage.CreateCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	if err := storage.SyncDomains(ctx, "c1", []model.RemoteDomain{{RemoteID: "d1", Name: "example.cn"}}); err != nil {
		t.Fatal(err)
	}
	domains, _ := storage.ListDomains(ctx)
	initial := []model.RemoteRecord{{RemoteID: "r1", Name: "@", Type: "A", Value: "192.0.2.1"}, {RemoteID: "r2", Name: "www", Type: "A", Value: "192.0.2.2"}}
	if err := storage.SyncRecords(ctx, domains[0].ID, initial); err != nil {
		t.Fatal(err)
	}
	before, _ := storage.ListRecords(ctx, domains[0].ID)
	ids := map[string]string{}
	for _, record := range before {
		ids[record.RemoteID] = record.ID
	}
	updated := []model.RemoteRecord{{RemoteID: "r1", Name: "@", Type: "A", Value: "192.0.2.10"}, {RemoteID: "r2", Name: "www", Type: "A", Value: "192.0.2.2"}}
	if err := storage.SyncRecords(ctx, domains[0].ID, updated); err != nil {
		t.Fatal(err)
	}
	after, _ := storage.ListRecords(ctx, domains[0].ID)
	for _, record := range after {
		if record.ID != ids[record.RemoteID] {
			t.Fatalf("local ID changed for %s: before=%s after=%s", record.RemoteID, ids[record.RemoteID], record.ID)
		}
	}
}
