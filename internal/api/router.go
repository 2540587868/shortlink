package api

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ysqss/shortlink/internal/analytics"
	"github.com/ysqss/shortlink/internal/config"
	"github.com/ysqss/shortlink/internal/core"
	"github.com/ysqss/shortlink/internal/metrics"
	"github.com/ysqss/shortlink/internal/model"
	"github.com/ysqss/shortlink/internal/store"

	qrcode "github.com/skip2/go-qrcode"
)

type Server struct {
	svc       core.Service
	store     *store.Store
	analytics *analytics.Analytics
	cfg       *config.Manager
	mux       *http.ServeMux
}

func NewServer(svc core.Service, st *store.Store, an *analytics.Analytics, cfg *config.Manager) *Server {
	s := &Server{
		svc:       svc,
		store:     st,
		analytics: an,
		cfg:       cfg,
		mux:       http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/v1/links", s.handleLinks)
	s.mux.HandleFunc("/api/v1/links/", s.handleLinkBySlug)
	s.mux.HandleFunc("/api/v1/links/batch", s.handleBatchCreate)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/health", s.handleHealth)

	s.mux.HandleFunc("/", s.handleRedirectOrAPI)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createLink(w, r)
	case http.MethodGet:
		s.listLinks(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleLinkBySlug(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r.URL.Path, "/api/v1/links/")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "slug is required"})
		return
	}

	if strings.HasSuffix(r.URL.Path, "/stats") {
		if r.Method == http.MethodGet {
			s.getStats(w, r, slug)
			return
		}
	}

	if strings.HasSuffix(r.URL.Path, "/stats/daily") {
		if r.Method == http.MethodGet {
			s.getDailyStats(w, r, slug)
			return
		}
	}

	if strings.HasSuffix(r.URL.Path, "/stats/top") {
		if r.Method == http.MethodGet {
			s.getTopStats(w, r, slug)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.getLink(w, r, slug)
	case http.MethodDelete:
		s.deleteLink(w, r, slug)
	case http.MethodPut:
		s.updateLink(w, r, slug)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req struct {
		Links []struct {
			LongURL    string `json:"long_url"`
			CustomSlug string `json:"custom_slug"`
		} `json:"links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	var results []map[string]any
	for _, linkReq := range req.Links {
		link, err := s.svc.Create(model.CreateOptions{CustomSlug: linkReq.CustomSlug}, linkReq.LongURL)
		if err != nil {
			results = append(results, map[string]any{"error": err.Error(), "long_url": linkReq.LongURL})
			continue
		}
		results = append(results, map[string]any{
			"slug":      link.Slug,
			"short_url": s.cfg.Get().Server.PublicURL + "/" + link.Slug,
			"long_url":  link.LongURL,
		})
	}

	writeJSON(w, http.StatusCreated, map[string]any{"results": results})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"dropped":  s.analytics.Dropped(),
		"inflight": s.analytics.Inflight(),
	})
}

func (s *Server) handleRedirectOrAPI(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")

	if strings.HasSuffix(slug, "/verify") {
		s.handleVerifyPassword(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if slug == "" || strings.HasPrefix(slug, "api/") || slug == "health" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}

	slug = strings.SplitN(slug, "/", 2)[0]

	link, err := s.svc.Lookup(slug)
	if err != nil {
		switch err {
		case model.ErrSlugNotFound:
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "slug not found"})
		case model.ErrLinkExpired:
			writeJSON(w, http.StatusGone, map[string]any{"error": "link expired"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
		}
		return
	}

	if link.Password != "" {
		authCookie, err := r.Cookie("sl_auth_" + slug)
		if err != nil || authCookie.Value != link.Password {
			s.servePasswordPage(w, slug)
			return
		}
	}

	s.analytics.Record(&model.ClickEvent{
		Slug:      slug,
		Timestamp: time.Now(),
		Referer:   r.Header.Get("Referer"),
		UserAgent: r.Header.Get("User-Agent"),
		IP:        extractIP(r),
	})

	metrics.RedirectsTotal.Inc()

	w.Header().Set("Location", link.LongURL)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusFound)
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LongURL    string `json:"long_url"`
		CustomSlug string `json:"custom_slug"`
		Password   string `json:"password"`
		TTLSeconds int64  `json:"ttl_seconds"`
		GenerateQR bool   `json:"generate_qr"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	var ttl time.Duration
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	opts := model.CreateOptions{
		CustomSlug: req.CustomSlug,
		Password:   req.Password,
		TTL:        ttl,
	}

	link, err := s.svc.Create(opts, req.LongURL)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrSlugExists {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	var expiresAt string
	if link.ExpiresAt != nil {
		expiresAt = link.ExpiresAt.Format(time.RFC3339)
	}

	resp := map[string]any{
		"slug":       link.Slug,
		"short_url":  s.cfg.Get().Server.PublicURL + "/" + link.Slug,
		"long_url":   link.LongURL,
		"expires_at": expiresAt,
		"created_at": link.CreatedAt.Format(time.RFC3339),
	}

	if req.GenerateQR {
		shortURL := s.cfg.Get().Server.PublicURL + "/" + link.Slug
		var png []byte
		png, err := qrcode.Encode(shortURL, qrcode.Medium, 256)
		if err != nil {
			slog.Error("failed to generate QR code", "error", err)
		} else {
			resp["qr_code"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}

	writeJSON(w, http.StatusCreated, resp)
	metrics.ShortLinksCreated.Inc()
}

func (s *Server) getLink(w http.ResponseWriter, r *http.Request, slug string) {
	link, err := s.svc.Lookup(slug)
	if err != nil {
		status := http.StatusNotFound
		if err == model.ErrLinkExpired {
			status = http.StatusGone
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	total, _ := s.store.GetClickStats(slug)

	var expiresAt string
	if link.ExpiresAt != nil {
		expiresAt = link.ExpiresAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":         link.Slug,
		"short_url":    s.cfg.Get().Server.PublicURL + "/" + link.Slug,
		"long_url":     link.LongURL,
		"has_password": link.Password != "",
		"expires_at":   expiresAt,
		"created_at":   link.CreatedAt.Format(time.RFC3339),
		"clicks":       total,
	})
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	search := r.URL.Query().Get("search")

	if search != "" {
		links, err := s.svc.Search(search)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"links": formatLinks(s.cfg.Get().Server.PublicURL, links),
			"total": len(links),
		})
		return
	}

	links, total, err := s.svc.List(page, pageSize)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"links":     formatLinks(s.cfg.Get().Server.PublicURL, links),
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request, slug string) {
	if err := s.svc.Delete(slug); err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrSlugNotFound {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "deleted"})
}

func (s *Server) updateLink(w http.ResponseWriter, r *http.Request, slug string) {
	var req struct {
		LongURL    string `json:"long_url"`
		Password   string `json:"password"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	var ttl time.Duration
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	opts := model.CreateOptions{
		Password: req.Password,
		TTL:      ttl,
	}

	link, err := s.svc.Update(slug, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":      link.Slug,
		"short_url": s.cfg.Get().Server.PublicURL + "/" + link.Slug,
		"long_url":  link.LongURL,
	})
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request, slug string) {
	if slug == "" {
		s.handleStats(w, r)
		return
	}

	total, err := s.store.GetClickStats(slug)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":  slug,
		"total": total,
	})
}

func (s *Server) getDailyStats(w http.ResponseWriter, r *http.Request, slug string) {
	daily, err := s.store.GetDailyClickStats(slug)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var result []map[string]any
	for date, count := range daily {
		result = append(result, map[string]any{"date": date, "count": count})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":  slug,
		"daily": result,
	})
}

func (s *Server) getTopStats(w http.ResponseWriter, r *http.Request, slug string) {
	limit := 10

	referers, err := s.store.GetTopReferers(slug, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	userAgents, err := s.store.GetTopUserAgents(slug, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":         slug,
		"top_referers": referers,
		"top_user_agents": userAgents,
	})
}

func (s *Server) servePasswordPage(w http.ResponseWriter, slug string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	passwordPageHTML := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Password Required - ShortLink</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
.card { background: #fff; border-radius: 12px; box-shadow: 0 4px 24px rgba(0,0,0,0.1); padding: 40px; max-width: 400px; width: 90%; text-align: center; }
h1 { font-size: 24px; color: #333; margin-bottom: 8px; }
p { color: #666; margin-bottom: 24px; font-size: 14px; }
input[type="password"] { width: 100%; padding: 12px 16px; border: 2px solid #e0e0e0; border-radius: 8px; font-size: 16px; outline: none; transition: border-color 0.2s; }
input[type="password"]:focus { border-color: #4f46e5; }
button { width: 100%; padding: 12px; background: #4f46e5; color: #fff; border: none; border-radius: 8px; font-size: 16px; cursor: pointer; margin-top: 16px; transition: background 0.2s; }
button:hover { background: #4338ca; }
.error { color: #dc2626; font-size: 14px; margin-top: 12px; display: none; }
</style>
</head>
<body>
<div class="card">
<h1>Password Required</h1>
<p>This short link is password-protected. Please enter the password to continue.</p>
<form id="pw-form">
<input type="password" id="password" placeholder="Enter password" autofocus required>
<button type="submit">Continue</button>
<p class="error" id="error">Incorrect password. Please try again.</p>
</form>
</div>
<script>
document.getElementById('pw-form').addEventListener('submit', async (e) => {
e.preventDefault();
const pw = document.getElementById('password').value;
const resp = await fetch('/` + slug + `/verify', {
method: 'POST',
headers: { 'Content-Type': 'application/json' },
body: JSON.stringify({ password: pw })
});
if (resp.ok) {
window.location.href = '/` + slug + `';
} else {
document.getElementById('error').style.display = 'block';
}
});
</script>
</body>
</html>`
	w.Write([]byte(passwordPageHTML))
}

func (s *Server) handleVerifyPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/")
	slug = strings.TrimSuffix(slug, "/verify")

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	valid, err := s.svc.VerifyPassword(slug, req.Password)
	if err != nil {
		if err == model.ErrSlugNotFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "slug not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}

	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid password"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "sl_auth_" + slug,
		Value:    req.Password,
		Path:     "/" + slug,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	writeJSON(w, http.StatusOK, map[string]any{"message": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func extractSlug(path, prefix string) string {
	slug := strings.TrimPrefix(path, prefix)
	slug = strings.TrimSuffix(slug, "/stats")
	slug = strings.TrimSuffix(slug, "/stats/daily")
	slug = strings.TrimSuffix(slug, "/")
	return slug
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.SplitN(xff, ",", 2)[0]
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

func formatLinks(publicURL string, links []*model.ShortLink) []map[string]any {
	var result []map[string]any
	for _, link := range links {
		result = append(result, map[string]any{
			"slug":       link.Slug,
			"short_url":  publicURL + "/" + link.Slug,
			"long_url":   link.LongURL,
			"created_at": link.CreatedAt.Format(time.RFC3339),
		})
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler, cfg *config.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			token := r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")

			c := cfg.Get()
			if c.Server.AdminToken != "" && token != c.Server.AdminToken {
				valid := false
				for _, t := range c.Auth.Tokens {
					if token == t {
						valid = true
						break
					}
				}
				if !valid {
					writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
		slog.Debug("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func ApplyMiddleware(handler http.Handler, cfg *config.Manager) http.Handler {
	h := handler
	h = corsMiddleware(h)
	h = loggingMiddleware(h)
	h = authMiddleware(h, cfg)
	h = metricsMiddleware(h)
	return h
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		path := normalizePath(r.URL.Path)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.statusCode)).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "/api/v1/links/") {
		if strings.HasSuffix(path, "/stats/daily") {
			return "/api/v1/links/:slug/stats/daily"
		}
		if strings.HasSuffix(path, "/stats") {
			return "/api/v1/links/:slug/stats"
		}
		return "/api/v1/links/:slug"
	}
	if strings.HasPrefix(path, "/api/") {
		return "/api/*"
	}
	if path == "/health" {
		return "/health"
	}
	return "/:slug"
}