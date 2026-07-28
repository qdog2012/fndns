package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fndns/manager/internal/model"
	"github.com/fndns/manager/internal/provider"
	"github.com/fndns/manager/internal/service"
	"github.com/fndns/manager/internal/store"
)

type Server struct {
	service *service.Service
	logger  *slog.Logger
	assets  fs.FS
}

func New(svc *service.Service, logger *slog.Logger, assets fs.FS) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	server := &Server{service: svc, logger: logger, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("GET /api/v1/overview", server.overview)
	mux.HandleFunc("GET /api/v1/capabilities/{provider}", server.capability)
	mux.HandleFunc("GET /api/v1/credentials", server.listCredentials)
	mux.HandleFunc("POST /api/v1/credentials", server.createCredential)
	mux.HandleFunc("PUT /api/v1/credentials/{credentialID}", server.updateCredential)
	mux.HandleFunc("DELETE /api/v1/credentials/{credentialID}", server.deleteCredential)
	mux.HandleFunc("POST /api/v1/credentials/{credentialID}/refresh", server.refreshCredential)
	mux.HandleFunc("POST /api/v1/refresh", server.refreshAll)
	mux.HandleFunc("GET /api/v1/domains", server.listDomains)
	mux.HandleFunc("POST /api/v1/domains/{domainID}/refresh", server.refreshDomain)
	mux.HandleFunc("GET /api/v1/domains/{domainID}/records", server.listRecords)
	mux.HandleFunc("POST /api/v1/domains/{domainID}/records", server.createRecord)
	mux.HandleFunc("PUT /api/v1/domains/{domainID}/records/{recordID}", server.updateRecord)
	mux.HandleFunc("DELETE /api/v1/domains/{domainID}/records/{recordID}", server.deleteRecord)
	mux.HandleFunc("POST /api/v1/domains/{domainID}/records/{recordID}/status", server.setRecordStatus)
	mux.HandleFunc("POST /api/v1/domains/{domainID}/records/batch-delete", server.batchDelete)
	mux.HandleFunc("POST /api/v1/domains/{domainID}/records/batch-status", server.batchStatus)
	mux.HandleFunc("GET /api/v1/logs", server.listLogs)
	mux.HandleFunc("GET /", server.static)
	return server.middleware(mux)
}

type response struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{Data: data})
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, store.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, service.ErrConflict):
		status, code = http.StatusConflict, "remote_conflict"
	case errors.Is(err, service.ErrExpiredCache):
		status, code = http.StatusConflict, "cache_expired"
	case errors.Is(err, provider.ErrUnsupported):
		status, code = http.StatusUnprocessableEntity, "unsupported"
	default:
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			status, code = http.StatusRequestEntityTooLarge, "payload_too_large"
		} else if !strings.Contains(err.Error(), "数据库") && !strings.Contains(err.Error(), "SQL") {
			status, code = http.StatusBadRequest, "invalid_request"
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{Error: &apiError{Code: code, Message: err.Error()}})
}

func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求数据格式错误: %w", err)
	}
	return nil
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'")
		if isMutation(r.Method) && !validOrigin(r) {
			writeError(w, errors.New("请求来源校验失败"))
			return
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("request panic", "error", recovered)
				writeError(w, errors.New("服务器内部错误"))
			}
			s.logger.Debug("request", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(started))
		}()
		next.ServeHTTP(w, r)
	})
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(origin), "://"+strings.ToLower(r.Host))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now()})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.Store().Overview(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) capability(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.Capability(r.PathValue("provider"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListCredentials(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	var input model.CredentialInput
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := s.service.AddCredential(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateCredential(w http.ResponseWriter, r *http.Request) {
	var input model.CredentialInput
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := s.service.UpdateCredential(r.Context(), r.PathValue("credentialID"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteCredential(r.Context(), r.PathValue("credentialID")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) refreshCredential(w http.ResponseWriter, r *http.Request) {
	if err := s.service.RefreshCredential(r.Context(), r.PathValue("credentialID")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"refreshed": true})
}

func (s *Server) refreshAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.RefreshAll(r.Context()))
}

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListDomains(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) refreshDomain(w http.ResponseWriter, r *http.Request) {
	if err := s.service.RefreshDomain(r.Context(), r.PathValue("domainID")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"refreshed": true})
}

func (s *Server) listRecords(w http.ResponseWriter, r *http.Request) {
	domain, records, err := s.service.ListRecords(r.Context(), r.PathValue("domainID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": domain, "records": records})
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request) {
	var input model.RecordInput
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := s.service.CreateRecord(r.Context(), r.PathValue("domainID"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request) {
	var input model.RecordInput
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := s.service.UpdateRecord(r.Context(), r.PathValue("domainID"), r.PathValue("recordID"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteRecord(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteRecord(r.Context(), r.PathValue("domainID"), r.PathValue("recordID")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) setRecordStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := s.service.SetRecordStatus(r.Context(), r.PathValue("domainID"), r.PathValue("recordID"), input.Enabled); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) batchDelete(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RecordIDs []string `json:"recordIds"`
	}
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	if len(input.RecordIDs) == 0 || len(input.RecordIDs) > 100 {
		writeError(w, errors.New("请选择 1 至 100 条记录"))
		return
	}
	writeJSON(w, http.StatusOK, s.service.BatchDelete(r.Context(), r.PathValue("domainID"), input.RecordIDs))
}

func (s *Server) batchStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RecordIDs []string `json:"recordIds"`
		Enabled   bool     `json:"enabled"`
	}
	if err := decode(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	if len(input.RecordIDs) == 0 || len(input.RecordIDs) > 100 {
		writeError(w, errors.New("请选择 1 至 100 条记录"))
		return
	}
	writeJSON(w, http.StatusOK, s.service.BatchStatus(r.Context(), r.PathValue("domainID"), input.RecordIDs, input.Enabled))
}

func parseDate(value string, endOfDay bool) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return &parsed
}

func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	filter := model.LogFilter{From: parseDate(r.URL.Query().Get("from"), false), To: parseDate(r.URL.Query().Get("to"), true),
		CredentialID: r.URL.Query().Get("credentialId"), Domain: r.URL.Query().Get("domain"),
		Action: r.URL.Query().Get("action"), Result: r.URL.Query().Get("result"), Limit: limit, Offset: offset}
	items, err := s.service.Store().ListLogs(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	content, err := fs.ReadFile(s.assets, name)
	if err != nil {
		content, err = fs.ReadFile(s.assets, "index.html")
		name = "index.html"
	}
	if err != nil {
		http.Error(w, "前端资源未构建", http.StatusServiceUnavailable)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.Contains(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
