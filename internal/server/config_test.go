package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	body := []byte("listen_addr: :9090\npublic_base_url: https://usage.example.test\napi_key: secret\nsqlite_path: ./usage.db\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Fatalf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.PublicBaseURL != "https://usage.example.test" {
		t.Fatalf("PublicBaseURL = %q, want https://usage.example.test", cfg.PublicBaseURL)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("APIKey = %q, want secret", cfg.APIKey)
	}
	if cfg.SQLitePath != "./usage.db" {
		t.Fatalf("SQLitePath = %q, want ./usage.db", cfg.SQLitePath)
	}
}

func TestValidateConfig(t *testing.T) {
	valid := ServerConfig{
		APIKey:     "secret",
		SQLitePath: "usage.db",
	}
	if err := ValidateConfig(&valid); err != nil {
		t.Fatalf("ValidateConfig(valid) error = %v", err)
	}
	if valid.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want default :8080", valid.ListenAddr)
	}

	tests := []struct {
		name string
		cfg  ServerConfig
	}{
		{name: "api key required", cfg: ServerConfig{SQLitePath: "usage.db"}},
		{name: "sqlite path required", cfg: ServerConfig{APIKey: "secret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateConfig(&tt.cfg); err == nil {
				t.Fatal("ValidateConfig() error = nil, want error")
			}
		})
	}
}
