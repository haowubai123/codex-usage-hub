package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		home    string
		appData string
		want    string
	}{
		{
			name:    "windows uses appdata",
			goos:    "windows",
			home:    `C:\Users\a`,
			appData: `C:\Users\a\AppData\Roaming`,
			want:    `C:\Users\a\AppData\Roaming\codex-usage-client\config.yaml`,
		},
		{
			name: "windows falls back to home roaming path",
			goos: "windows",
			home: `C:\Users\a`,
			want: `C:\Users\a\AppData\Roaming\codex-usage-client\config.yaml`,
		},
		{
			name: "macos application support",
			goos: "darwin",
			home: "/Users/a",
			want: "/Users/a/Library/Application Support/codex-usage-client/config.yaml",
		},
		{
			name: "linux xdg config",
			goos: "linux",
			home: "/home/a",
			want: "/home/a/.config/codex-usage-client/config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultConfigPath(tt.goos, tt.home, tt.appData); got != tt.want {
				t.Fatalf("DefaultConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveCodexHomePrefersConfig(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{CodexHome: `D:\codex-home`}, map[string]string{"CODEX_HOME": `D:\env-home`}, `C:\Users\a`)
	if got != `D:\codex-home` {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCodexHomeUsesEnv(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{}, map[string]string{"CODEX_HOME": `/tmp/codex`}, `/home/a`)
	if got != `/tmp/codex` {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCodexHomeUsesDefault(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{}, map[string]string{}, `/home/a`)
	if got != `/home/a/.codex` {
		t.Fatalf("got %q", got)
	}
}

func TestLoadConfigParsesScanInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := strings.Join([]string{
		"server_url: https://usage.example.test",
		"api_key: secret",
		"identity_key: alice",
		"scan_interval: 30s",
		"codex_home: /tmp/codex",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.ScanInterval != 30*time.Second {
		t.Fatalf("ScanInterval = %s, want 30s", cfg.ScanInterval)
	}
	if cfg.RawInterval != "30s" {
		t.Fatalf("RawInterval = %q, want 30s", cfg.RawInterval)
	}
}

func TestLoadConfigDefaultsScanInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := strings.Join([]string{
		"server_url: https://usage.example.test",
		"api_key: secret",
		"identity_key: alice",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.ScanInterval != 5*time.Minute {
		t.Fatalf("ScanInterval = %s, want 5m", cfg.ScanInterval)
	}
}

func TestValidateConfig(t *testing.T) {
	valid := ClientConfig{
		ServerURL:    "https://usage.example.test",
		APIKey:       "secret",
		IdentityKey:  "alice",
		ScanInterval: time.Minute,
	}
	if err := ValidateConfig(valid); err != nil {
		t.Fatalf("ValidateConfig(valid) error = %v", err)
	}

	tests := []struct {
		name string
		cfg  ClientConfig
	}{
		{name: "server url required", cfg: ClientConfig{APIKey: "secret", IdentityKey: "alice", ScanInterval: time.Minute}},
		{name: "api key required", cfg: ClientConfig{ServerURL: "https://usage.example.test", IdentityKey: "alice", ScanInterval: time.Minute}},
		{name: "identity key required", cfg: ClientConfig{ServerURL: "https://usage.example.test", APIKey: "secret", ScanInterval: time.Minute}},
		{name: "positive interval required", cfg: ClientConfig{ServerURL: "https://usage.example.test", APIKey: "secret", IdentityKey: "alice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateConfig(tt.cfg); err == nil {
				t.Fatal("ValidateConfig() error = nil, want error")
			}
		})
	}
}
