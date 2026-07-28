package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fndns/manager/internal/model"
)

func TestCloudflareListRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"r1","name":"www.example.com","type":"A","content":"192.0.2.1","ttl":300,"proxied":true,"proxiable":true}],"result_info":{"page":1,"total_pages":1}}`))
	}))
	defer server.Close()
	client := NewCloudflare(server.Client(), "test-token")
	client.baseURL = server.URL
	records, err := client.ListRecords(context.Background(), model.Domain{RemoteID: "z1", Name: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "www" || !records[0].Proxied {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestCloudflareSRVValidation(t *testing.T) {
	_, err := cfRecordBody(model.Domain{Name: "example.com"}, model.RecordInput{Name: "_https._tcp", Type: "SRV", Value: "invalid"})
	if err == nil {
		t.Fatal("expected SRV validation error")
	}
}
