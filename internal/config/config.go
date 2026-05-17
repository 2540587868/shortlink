package config

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Short    ShortConfig    `yaml:"short"`
	Auth     AuthConfig     `yaml:"auth"`
}

type ServerConfig struct {
	Listen     string `yaml:"listen"`
	PublicURL  string `yaml:"public_url"`
	AdminToken string `yaml:"admin_token"`
	DebugPort  string `yaml:"debug_port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type ShortConfig struct {
	DefaultTTLSeconds int64 `yaml:"default_ttl_seconds"`
	CacheSize         int   `yaml:"cache_size"`
}

type AuthConfig struct {
	Tokens []string `yaml:"tokens"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:     ":8080",
			PublicURL:  "http://localhost:8080",
			AdminToken: "changeme",
		},
		Database: DatabaseConfig{
			Path: "data/shortlink.db",
		},
		Short: ShortConfig{
			DefaultTTLSeconds: 0,
			CacheSize:         10000,
		},
	}
}

type Manager struct {
	current atomic.Value
	path    string
}

func Load(path string) (*Manager, error) {
	m := &Manager{path: path}

	cfg := Default()
	if path != "" {
		if err := m.loadFile(cfg); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	m.current.Store(cfg)
	return m, nil
}

func (m *Manager) loadFile(cfg *Config) error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

func (m *Manager) Get() *Config {
	v, ok := m.current.Load().(*Config)
	if !ok {
		return Default()
	}
	return v
}

func (m *Manager) Reload() error {
	cfg := Default()
	if err := m.loadFile(cfg); err != nil {
		return err
	}
	m.current.Store(cfg)
	return nil
}

func (c *Config) TTL() time.Duration {
	if c.Short.DefaultTTLSeconds <= 0 {
		return 0
	}
	return time.Duration(c.Short.DefaultTTLSeconds) * time.Second
}
