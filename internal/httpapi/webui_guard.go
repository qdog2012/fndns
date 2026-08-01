package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	webUISessionCookie = "fndns_webui_session"
	webUISessionHeader = "X-FNDNS-WebUI-Session"
)

type webUISessionContextKey struct{}

// RequireFNOSWebUI 只允许由受信任 FNOS WebUI 建立的浏览器会话。
// 直接在浏览器地址栏打开服务端口时，顶层导航不会获得会话令牌。
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
		// 哈希命名的前端静态资源不包含用户数据。跨站 iframe 环境下
		// SameSite Cookie 可能不会随资源请求发送，因此允许公开读取。
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasPrefix(r.URL.Path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}
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
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, withWebUISessionToken(r, token))
			return
		}

		if !hasWebUISession(r, token) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withWebUISessionToken(r *http.Request, token string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), webUISessionContextKey{}, token))
}

func webUISessionTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(webUISessionContextKey{}).(string)
	return token
}

func hasWebUISession(r *http.Request, token string) bool {
	provided := strings.TrimSpace(r.Header.Get(webUISessionHeader))
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
		return true
	}
	cookie, err := r.Cookie(webUISessionCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func isEmbeddedFNOSRequest(r *http.Request) bool {
	destination := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")))
	if destination != "iframe" && destination != "document" && destination != "" {
		return false
	}
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if destination == "iframe" && (site == "same-origin" || site == "same-site") {
		return true
	}
	// FNOS 内网入口可能使用 IP、设备名和应用端口的组合，浏览器会把
	// 这种 iframe 标记为 cross-site。此时通过父页面 Referer 验证它确实
	// 来自同一设备或受信任的 FNOS WebUI，而不是直接放行跨站 iframe。
	referer, err := url.Parse(r.Referer())
	if err != nil || referer.Hostname() == "" {
		return false
	}
	requestHost := r.Host
	if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
		requestHost = forwardedHost
	}
	if !isTrustedFNOSParent(referer, hostname(requestHost)) {
		return false
	}
	if destination == "iframe" {
		return true
	}
	return isFNOSNavigationParent(referer)
}

func isTrustedFNOSParent(referer *url.URL, requestHostname string) bool {
	parentHostname := strings.ToLower(strings.TrimSuffix(referer.Hostname(), "."))
	requestHostname = strings.ToLower(strings.TrimSuffix(requestHostname, "."))
	if parentHostname == "" || requestHostname == "" {
		return false
	}
	if parentHostname == requestHostname {
		return true
	}
	if parentHostname == "fnos.net" || strings.HasSuffix(parentHostname, ".fnos.net") {
		return true
	}
	return isLocalHostname(parentHostname) && isLocalHostname(requestHostname) && isFNOSWebUIPort(referer.Port())
}

func isLocalHostname(value string) bool {
	if ip := net.ParseIP(value); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	return value == "localhost" || !strings.Contains(value, ".") || strings.HasSuffix(value, ".local") || strings.HasSuffix(value, ".lan")
}

func isFNOSWebUIPort(port string) bool {
	return port == "5666" || port == "5667"
}

func isFNOSNavigationParent(referer *url.URL) bool {
	host := strings.ToLower(strings.TrimSuffix(referer.Hostname(), "."))
	return isFNOSWebUIPort(referer.Port()) || host == "fnos.net" || strings.HasSuffix(host, ".fnos.net")
}

func hostname(hostport string) string {
	parsed, err := url.Parse("//" + strings.TrimSpace(hostport))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
