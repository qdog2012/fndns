package httpapi

import (
	"net/http"
	"strings"
)

// Mount 兼容 FNOS 网关的两种转发形式：部分版本保留 gatewayPrefix，
// 部分版本会在把请求交给 Unix Socket 前剥离此前缀。
func Mount(basePath string, next http.Handler) http.Handler {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return next
	}
	basePath = "/" + strings.Trim(strings.ReplaceAll(basePath, "//", "/"), "/")
	stripped := http.StripPrefix(basePath, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == basePath {
			target := basePath + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		if strings.HasPrefix(r.URL.Path, basePath+"/") {
			stripped.ServeHTTP(w, r)
			return
		}
		// gatewayPrefix 已由 FNOS 剥离时，请求会以 /、/assets 或 /api 开头。
		// 直接交给应用路由，与 fnssh 的根路由回退行为保持一致。
		next.ServeHTTP(w, r)
	})
}

// RequireFNOSAdmin 可供明确注入管理员标记的 FNOS 环境选用。
// 正式 FPK 的 TCP 健康检查端口仅绑定回环地址，外部客户端无法连接。
func RequireFNOSAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Trim-Isadmin")), "true") {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "仅允许 FNOS 管理员访问", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
