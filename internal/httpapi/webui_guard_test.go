package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireFNOSWebUI(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Application-Path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requireFNOSWebUI(next, "test-token")

	t.Run("direct navigation is hidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("cross-site iframe is hidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("FNOS iframe creates session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		req.Header.Set("Sec-Fetch-Site", "same-site")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", response.Code)
		}
		cookie := response.Result().Cookies()[0]
		if cookie.Name != webUISessionCookie || cookie.Value != "test-token" || !cookie.HttpOnly {
			t.Fatalf("unexpected session cookie: %#v", cookie)
		}

		apiRequest := httptest.NewRequest(http.MethodGet, "http://nas:18788/api/v1/domains", nil)
		apiRequest.AddCookie(cookie)
		apiResponse := httptest.NewRecorder()
		handler.ServeHTTP(apiResponse, apiRequest)
		if apiResponse.Code != http.StatusNoContent || apiResponse.Header().Get("X-Application-Path") != "/api/v1/domains" {
			t.Fatalf("API status = %d, path = %q", apiResponse.Code, apiResponse.Header().Get("X-Application-Path"))
		}
	})

	t.Run("invalid session is hidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/assets/app.js", nil)
		req.AddCookie(&http.Cookie{Name: webUISessionCookie, Value: strings.Repeat("x", 10)})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})
}
