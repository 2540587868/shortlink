package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ysqss/shortlink/internal/analytics"
	"github.com/ysqss/shortlink/internal/cache"
	"github.com/ysqss/shortlink/internal/config"
	"github.com/ysqss/shortlink/internal/core"
	"github.com/ysqss/shortlink/internal/model"
	"github.com/ysqss/shortlink/internal/store"
	"gopkg.in/yaml.v3"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st, err := store.New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	c := cache.NewShardedLRU[string, *model.ShortLink](1000)
	svc := core.NewService(st, c)
	an := analytics.New(st, 64)

	cfgMgr := newTestConfigManager(t, dir, "", nil)

	return NewServer(svc, st, an, cfgMgr)
}

func newTestConfigManager(t *testing.T, dir, adminToken string, tokens []string) *config.Manager {
	t.Helper()

	cfg := config.Default()
	cfg.Server.PublicURL = "http://localhost:8080"
	cfg.Server.AdminToken = adminToken
	cfg.Auth.Tokens = tokens

	cfgPath := filepath.Join(dir, "config.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	mgr, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	return mgr
}

func doRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestCreateLink(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/very/long/path"}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeJSON(t, w)
	if resp["slug"] == "" {
		t.Error("expected non-empty slug")
	}
	if resp["long_url"] != "https://example.com/very/long/path" {
		t.Errorf("unexpected long_url: %v", resp["long_url"])
	}
}

func TestCreateLinkInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links", `{invalid}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateLinkEmptyURL(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links", `{"long_url": ""}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCreateLinkCustomSlug(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com", "custom_slug": "my-link"}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	if resp["slug"] != "my-link" {
		t.Errorf("expected slug 'my-link', got %v", resp["slug"])
	}
}

func TestCreateLinkDuplicateSlug(t *testing.T) {
	srv := newTestServer(t)

	doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com", "custom_slug": "dup"}`)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://other.com", "custom_slug": "dup"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestCreateLinkWithTTL(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com", "ttl_seconds": 3600}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	if resp["expires_at"] == "" {
		t.Error("expected non-empty expires_at")
	}
}

func TestListLinks(t *testing.T) {
	srv := newTestServer(t)

	doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/1"}`)
	doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/2"}`)

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/links", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	links, ok := resp["links"].([]any)
	if !ok {
		t.Fatal("expected links array")
	}
	if len(links) < 2 {
		t.Errorf("expected at least 2 links, got %d", len(links))
	}
}

func TestListLinksPagination(t *testing.T) {
	srv := newTestServer(t)

	for i := 0; i < 5; i++ {
		doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
			`{"long_url": "https://example.com/path"}`)
	}

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/links?page=1&page_size=2", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	if resp["page_size"].(float64) != 2 {
		t.Errorf("expected page_size 2, got %v", resp["page_size"])
	}
	links := resp["links"].([]any)
	if len(links) != 2 {
		t.Errorf("expected 2 links on page, got %d", len(links))
	}
}

func TestListLinksSearch(t *testing.T) {
	srv := newTestServer(t)

	doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://hello.example.com", "custom_slug": "hello"}`)
	doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://bye.example.com", "custom_slug": "bye"}`)

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/links?search=hello", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	links := resp["links"].([]any)
	if len(links) != 1 {
		t.Errorf("expected 1 search result, got %d", len(links))
	}
}

func TestGetLink(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/target"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/"+slug, "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp = decodeJSON(t, w)
	if resp["slug"] != slug {
		t.Errorf("expected slug %q, got %v", slug, resp["slug"])
	}
	if resp["long_url"] != "https://example.com/target" {
		t.Errorf("unexpected long_url: %v", resp["long_url"])
	}
}

func TestGetLinkNotFound(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteLink(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/delete-me"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodDelete, "/api/v1/links/"+slug, "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp = decodeJSON(t, w)
	if resp["message"] != "deleted" {
		t.Errorf("expected 'deleted', got %v", resp["message"])
	}

	w = doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/"+slug, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDeleteLinkNotFound(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodDelete, "/api/v1/links/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateLink(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/original", "password": "old"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodPut, "/api/v1/links/"+slug,
		`{"password": "new", "ttl_seconds": 7200}`)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateLinkNotFound(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPut, "/api/v1/links/nonexistent",
		`{"password": "test"}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRedirectSuccess(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/target"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodGet, "/"+slug, "")
	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://example.com/target" {
		t.Errorf("expected Location header, got %q", loc)
	}
}

func TestRedirectNotFound(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/nonexistent-slug", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRedirectRootPath(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for root, got %d", w.Code)
	}
}

func TestRedirectAPIPath(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/some-slug", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for api path on root handler, got %d", w.Code)
	}
}

func TestGetStatsEmptySlug(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodGet, "/api/v1/stats", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp := decodeJSON(t, w)
	if _, ok := resp["dropped"]; !ok {
		t.Error("expected 'dropped' in stats response")
	}
	if _, ok := resp["inflight"]; !ok {
		t.Error("expected 'inflight' in stats response")
	}
}

func TestGetLinkStats(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/stats-test"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/"+slug+"/stats", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp = decodeJSON(t, w)
	if resp["slug"] != slug {
		t.Errorf("expected slug %q, got %v", slug, resp["slug"])
	}
}

func BenchmarkAPIRedirect(b *testing.B) {
	srv := newTestServer(&testing.T{})
	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/bench"}`)
	resp := decodeJSON(&testing.T{}, w)
	slug := resp["slug"].(string)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doRequest(srv.Handler(), http.MethodGet, "/"+slug, "")
	}
}

func BenchmarkAPICreate(b *testing.B) {
	srv := newTestServer(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
			`{"long_url": "https://example.com/bench-create"}`)
	}
}

func BenchmarkAPIHealth(b *testing.B) {
	srv := newTestServer(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doRequest(srv.Handler(), http.MethodGet, "/health", "")
	}
}

func TestGetDailyStats(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/daily-test"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	w = doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/"+slug+"/stats/daily", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	resp = decodeJSON(t, w)
	if resp["slug"] != slug {
		t.Errorf("expected slug %q, got %v", slug, resp["slug"])
	}
	if _, ok := resp["daily"]; !ok {
		t.Error("expected 'daily' in response")
	}
}

func TestBatchCreate(t *testing.T) {
	srv := newTestServer(t)

	body := `{
		"links": [
			{"long_url": "https://example.com/1"},
			{"long_url": "https://example.com/2"}
		]
	}`
	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links/batch", body)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeJSON(t, w)
	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatal("expected results array")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestBatchCreateInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links/batch", `{invalid}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/api/v1/links"},
		{http.MethodPut, "/api/v1/links/batch"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := doRequest(srv.Handler(), tt.method, tt.path, "")
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405, got %d", w.Code)
			}
		})
	}
}

func TestCorsMiddleware(t *testing.T) {
	srv := newTestServer(t)
	handler := corsMiddleware(srv.Handler())

	w := doRequest(handler, http.MethodOptions, "/api/v1/links", "")
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %q", acao)
	}
}

func TestCorsMiddlewareGET(t *testing.T) {
	srv := newTestServer(t)
	handler := corsMiddleware(srv.Handler())

	w := doRequest(handler, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header on GET response")
	}
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "secret-token", nil)

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	w := doRequest(handler, http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "secret-token", nil)

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/links",
		strings.NewReader(`{"long_url": "https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddlewareAuthToken(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "admin", []string{"api-token-1"})

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	req.Header.Set("Authorization", "Bearer api-token-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "admin", nil)

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareSkipsNonAPI(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "admin", nil)

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	w := doRequest(handler, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for non-API path, got %d", w.Code)
	}
}

func TestAuthMiddlewareNoTokenConfig(t *testing.T) {
	srv := newTestServer(t)
	mgr := newTestConfigManager(t, t.TempDir(), "", nil)

	a := &Server{
		svc:       srv.svc,
		store:     srv.store,
		analytics: srv.analytics,
		cfg:       mgr,
		mux:       http.NewServeMux(),
	}
	a.registerRoutes()
	handler := authMiddleware(a.Handler(), mgr)

	w := doRequest(handler, http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com"}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 when no auth configured, got %d", w.Code)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	srv := newTestServer(t)
	handler := loggingMiddleware(srv.Handler())

	w := doRequest(handler, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestApplyMiddleware(t *testing.T) {
	srv := newTestServer(t)
	handler := ApplyMiddleware(srv.Handler(), srv.cfg)

	w := doRequest(handler, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with full middleware stack, got %d", w.Code)
	}
}

func TestExtractSlug(t *testing.T) {
	tests := []struct {
		path     string
		prefix   string
		expected string
	}{
		{"/api/v1/links/abc123", "/api/v1/links/", "abc123"},
		{"/api/v1/links/abc123/", "/api/v1/links/", "abc123"},
		{"/api/v1/links/abc123/stats", "/api/v1/links/", "abc123"},
		{"/api/v1/links/abc123/stats/daily", "/api/v1/links/", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := extractSlug(tt.path, tt.prefix)
			if result != tt.expected {
				t.Errorf("extractSlug(%q, %q) = %q, want %q", tt.path, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-Forwarded-For",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1, 10.0.0.2"},
			expected: "10.0.0.1",
		},
		{
			name:     "X-Real-IP",
			headers:  map[string]string{"X-Real-IP": "10.0.0.3"},
			expected: "10.0.0.3",
		},
		{
			name:     "RemoteAddr",
			headers:  map[string]string{},
			expected: "192.0.2.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := extractIP(req)
			if result != tt.expected {
				t.Errorf("extractIP() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatLinks(t *testing.T) {
	links := []*model.ShortLink{
		{
			Slug:      "test1",
			LongURL:   "https://example.com/1",
			CreatedAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	result := formatLinks("http://localhost:8080", links)
	if len(result) != 1 {
		t.Fatalf("expected 1 formatted link, got %d", len(result))
	}
	if result[0]["slug"] != "test1" {
		t.Errorf("expected slug 'test1', got %v", result[0]["slug"])
	}
	if result[0]["short_url"] != "http://localhost:8080/test1" {
		t.Errorf("unexpected short_url: %v", result[0]["short_url"])
	}
}

func TestFormatLinksEmpty(t *testing.T) {
	result := formatLinks("http://localhost:8080", nil)
	if result == nil {
		t.Error("expected non-nil result for empty links")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 formatted links, got %d", len(result))
	}
}

func TestGetLinkStatsFlat(t *testing.T) {
	srv := newTestServer(t)

	w := doRequest(srv.Handler(), http.MethodPost, "/api/v1/links",
		`{"long_url": "https://example.com/stats-flat"}`)
	resp := decodeJSON(t, w)
	slug := resp["slug"].(string)

	doRequest(srv.Handler(), http.MethodGet, "/"+slug, "")

	time.Sleep(100 * time.Millisecond)

	w = doRequest(srv.Handler(), http.MethodGet, "/api/v1/links/"+slug+"/stats", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp = decodeJSON(t, w)
	if resp["slug"] != slug {
		t.Errorf("expected slug %q, got %v", slug, resp["slug"])
	}
}