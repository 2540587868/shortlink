package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.Listen != ":8080" {
		t.Errorf("expected :8080, got %q", cfg.Server.Listen)
	}
	if cfg.Server.PublicURL != "http://localhost:8080" {
		t.Errorf("expected http://localhost:8080, got %q", cfg.Server.PublicURL)
	}
	if cfg.Server.AdminToken != "changeme" {
		t.Errorf("expected changeme, got %q", cfg.Server.AdminToken)
	}
	if cfg.Database.Path != "data/shortlink.db" {
		t.Errorf("expected data/shortlink.db, got %q", cfg.Database.Path)
	}
	if cfg.Short.DefaultTTLSeconds != 0 {
		t.Errorf("expected 0, got %d", cfg.Short.DefaultTTLSeconds)
	}
	if cfg.Short.CacheSize != 10000 {
		t.Errorf("expected 10000, got %d", cfg.Short.CacheSize)
	}
}

func TestDefaultTTL(t *testing.T) {
	cfg := Default()
	if cfg.TTL() != 0 {
		t.Errorf("expected 0 TTL by default, got %v", cfg.TTL())
	}

	cfg.Short.DefaultTTLSeconds = 3600
	if cfg.TTL() != 1*time.Hour {
		t.Errorf("expected 1h TTL, got %v", cfg.TTL())
	}
}

func TestLoadEmptyPath(t *testing.T) {
	mgr, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg := mgr.Get()
	if cfg.Server.Listen != ":8080" {
		t.Errorf("expected default listen, got %q", cfg.Server.Listen)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := `
server:
  listen: ":9090"
  public_url: "https://short.example.com"
  admin_token: "test-token"
  debug_port: ":6060"
database:
  path: "data/test.db"
short:
  default_ttl_seconds: 7200
  cache_size: 5000
auth:
  tokens:
    - "token-a"
    - "token-b"
`
	_ = os.WriteFile(path, []byte(data), 0644)

	mgr, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cfg := mgr.Get()
	if cfg.Server.Listen != ":9090" {
		t.Errorf("expected :9090, got %q", cfg.Server.Listen)
	}
	if cfg.Server.PublicURL != "https://short.example.com" {
		t.Errorf("expected https://short.example.com, got %q", cfg.Server.PublicURL)
	}
	if cfg.Server.AdminToken != "test-token" {
		t.Errorf("expected test-token, got %q", cfg.Server.AdminToken)
	}
	if cfg.Server.DebugPort != ":6060" {
		t.Errorf("expected :6060, got %q", cfg.Server.DebugPort)
	}
	if cfg.Database.Path != "data/test.db" {
		t.Errorf("expected data/test.db, got %q", cfg.Database.Path)
	}
	if cfg.Short.DefaultTTLSeconds != 7200 {
		t.Errorf("expected 7200, got %d", cfg.Short.DefaultTTLSeconds)
	}
	if cfg.Short.CacheSize != 5000 {
		t.Errorf("expected 5000, got %d", cfg.Short.CacheSize)
	}
	if len(cfg.Auth.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(cfg.Auth.Tokens))
	}
	if cfg.Auth.Tokens[0] != "token-a" {
		t.Errorf("expected token-a, got %q", cfg.Auth.Tokens[0])
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_ = os.WriteFile(path, []byte(`server:
  listen: ":1111"
  public_url: "http://old.example.com"
  admin_token: "old"
database:
  path: "data/old.db"
`), 0644)

	mgr, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cfg := mgr.Get()
	if cfg.Server.Listen != ":1111" {
		t.Errorf("expected :1111, got %q", cfg.Server.Listen)
	}

	_ = os.WriteFile(path, []byte(`server:
  listen: ":2222"
  public_url: "http://new.example.com"
  admin_token: "new"
database:
  path: "data/new.db"
`), 0644)

	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	cfg = mgr.Get()
	if cfg.Server.Listen != ":2222" {
		t.Errorf("expected :2222 after reload, got %q", cfg.Server.Listen)
	}
	if cfg.Server.AdminToken != "new" {
		t.Errorf("expected new, got %q", cfg.Server.AdminToken)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("nonexistent-config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReloadPreservesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_ = os.WriteFile(path, []byte(`server:
  listen: ":3333"
  public_url: "http://partial.example.com"
`), 0644)

	mgr, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cfg := mgr.Get()
	if cfg.Database.Path != "data/shortlink.db" {
		t.Errorf("expected default db path, got %q", cfg.Database.Path)
	}
	if cfg.Short.CacheSize != 10000 {
		t.Errorf("expected default cache size, got %d", cfg.Short.CacheSize)
	}
}
