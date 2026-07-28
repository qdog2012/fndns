package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const webUISessionCookie = "fndns_webui_session"

// RequireFNOSWebUI 只允许由同站 FNOS WebUI iframe 建立的浏览器会话。
// 直接在浏览器地址栏打开服务端口时，顶层导航不会获得会话 Cookie。
func RequireFNOSWebUI(next http.Handler) (http.Handler, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("生成 WebUI 会话密钥失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	return requireFNOSWebUI(next, token), nil
}

func requireFNOSWebUI(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			if !isEmbeddedFNOSRequest(r) {
				http.NotFound(w, r)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     webUISessionCookie,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(webUISessionCookie)
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) != 1 {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isEmbeddedFNOSRequest(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")), "iframe") {
		return false
	}
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if site == "same-origin" || site == "same-site" {
		return true
	}
	// 兼容不发送 Sec-Fetch-Site、但保留 Referer 的旧版 WebView。
	if site != "" {
		return false
	}
	referer, err := url.Parse(r.Referer())
	if err != nil || referer.Hostname() == "" {
		return false
	}
	requestHost := r.Host
	if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
		requestHost = forwardedHost
	}
	return strings.EqualFold(referer.Hostname(), hostname(requestHost))
}

func hostname(hostport string) string {
	parsed, err := url.Parse("//" + strings.TrimSpace(hostport))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
