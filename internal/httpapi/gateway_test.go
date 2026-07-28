package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireFNOSAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequireFNOSAdmin(next)

	for _, test := range []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusForbidden},
		{name: "non-admin", header: "false", wantStatus: http.StatusForbidden},
		{name: "admin", header: "true", wantStatus: http.StatusNoContent},
		{name: "case-insensitive", header: "TRUE", wantStatus: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				req.Header.Set("X-Trim-Isadmin", test.header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestMount(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Stripped-Path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Mount("/app/fndns-manager", next)

	req := httptest.NewRequest(http.MethodGet, "/app/fndns-manager/api/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent || response.Header().Get("X-Stripped-Path") != "/api/health" {
		t.Fatalf("mounted request = status %d, path %q", response.Code, response.Header().Get("X-Stripped-Path"))
	}

	req = httptest.NewRequest(http.MethodGet, "/app/fndns-manager", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusTemporaryRedirect || response.Header().Get("Location") != "/app/fndns-manager/" {
		t.Fatalf("redirect = status %d, location %q", response.Code, response.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent || response.Header().Get("X-Stripped-Path") != "/api/health" {
		t.Fatalf("gateway-stripped request = status %d, path %q", response.Code, response.Header().Get("X-Stripped-Path"))
	}
}
