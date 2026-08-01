package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRequireFNOSWebUI(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Application-Path", r.URL.Path)
		w.Header().Set("X-WebUI-Session", webUISessionTokenFromContext(r.Context()))
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

	t.Run("untrusted cross-site iframe is hidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Referer", "https://evil.example/")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("local FNOS cross-site iframe creates session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://192.168.31.52:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Referer", "http://192.168.31.52:5666/")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent || response.Header().Get("X-WebUI-Session") != "test-token" {
			t.Fatalf("status = %d, session = %q", response.Code, response.Header().Get("X-WebUI-Session"))
		}
	})

	t.Run("local FNOS document-style app window creates session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://192.168.31.52:18788/", nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Site", "same-site")
		req.Header.Set("Referer", "http://192.168.31.52:5666/")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent || response.Header().Get("X-WebUI-Session") != "test-token" {
			t.Fatalf("status = %d, session = %q", response.Code, response.Header().Get("X-WebUI-Session"))
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

		headerRequest := httptest.NewRequest(http.MethodGet, "http://nas:18788/api/v1/domains", nil)
		headerRequest.Header.Set(webUISessionHeader, "test-token")
		headerResponse := httptest.NewRecorder()
		handler.ServeHTTP(headerResponse, headerRequest)
		if headerResponse.Code != http.StatusNoContent {
			t.Fatalf("header API status = %d", headerResponse.Code)
		}
	})

	t.Run("hashed assets are readable without cross-site cookies", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/assets/app.js", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", response.Code)
		}
	})

	t.Run("invalid session is hidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://nas:18788/api/v1/domains", nil)
		req.AddCookie(&http.Cookie{Name: webUISessionCookie, Value: strings.Repeat("x", 10)})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})
}

func TestStaticInjectsWebUISessionMeta(t *testing.T) {
	server := &Server{assets: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html><head></head><body></body></html>")},
	}}
	req := httptest.NewRequest(http.MethodGet, "http://nas:18788/", nil)
	req = withWebUISessionToken(req, "test-token")
	response := httptest.NewRecorder()
	server.static(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<meta name="fndns-webui-session" content="test-token" />`) {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}
