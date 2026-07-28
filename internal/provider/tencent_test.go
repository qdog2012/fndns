package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fndns/manager/internal/model"
)

func TestTencentSignsAndListsDomains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TC-Action") != "DescribeDomainList" {
			t.Fatalf("unexpected action: %s", r.Header.Get("X-TC-Action"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=AKIDTEST/") {
			t.Fatalf("unexpected authorization: %s", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"DomainList":[{"DomainId":42,"Name":"example.cn","Status":"ENABLE","Grade":"DP_FREE","RecordCount":3}],"DomainCountInfo":{"AllTotal":1},"RequestId":"req-1"}}`))
	}))
	defer server.Close()
	client := NewTencent(server.Client(), "AKIDTEST", "secret")
	client.endpoint = server.URL
	client.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	domains, err := client.ListDomains(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0].RemoteID != "42" || domains[0].Name != "example.cn" || domains[0].RecordCount != 3 {
		t.Fatalf("unexpected domains: %#v", domains)
	}
}

func TestTencentSRVValueRoundTrip(t *testing.T) {
	record := fromTCRecord(tcRecord{RecordID: 1, Name: "_sip._tcp", Type: "SRV", Value: "20 5060 sip.example.com", Status: "ENABLE", TTL: 600})
	if record.Weight != 20 || record.Value != "5060 sip.example.com" {
		t.Fatalf("unexpected parsed SRV record: %#v", record)
	}
	payload := tcRecordInput(model.Domain{Name: "example.com"}, model.RecordInput{Name: "_sip._tcp", Type: "SRV", Value: "5060 sip.example.com", Weight: 20, MX: 10})
	if payload["Value"] != "20 5060 sip.example.com" {
		t.Fatalf("unexpected SRV payload: %#v", payload)
	}
	if _, exists := payload["Weight"]; exists {
		t.Fatal("SRV weight must be encoded in Value, not DNSPod load-balancing Weight")
	}
}
