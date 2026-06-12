package client

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ClientConfig struct {
	ServerURL    string        `yaml:"server_url"`
	APIKey       string        `yaml:"api_key"`
	IdentityKey  string        `yaml:"identity_key"`
	ScanInterval time.Duration `yaml:"-"`
	RawInterval  string        `yaml:"scan_interval"`
	CodexHome    string        `yaml:"codex_home"`
}

func DefaultConfigPath(goos string, home string, appData string) string {
	if goos == "windows" {
		base := appData
		if base == "" {
			base = joinWithSeparator(`\`, home, "AppData", "Roaming")
		}
		return joinWithSeparator(`\`, base, "codex-usage-client", "config.yaml")
	}
	if goos == "darwin" {
		return joinWithSeparator("/", home, "Library", "Application Support", "codex-usage-client", "config.yaml")
	}
	return joinWithSeparator("/", home, ".config", "codex-usage-client", "config.yaml")
}

func LoadConfig(path string) (ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ClientConfig{}, err
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ClientConfig{}, err
	}
	if cfg.RawInterval == "" {
		cfg.ScanInterval = 5 * time.Minute
		return cfg, nil
	}

	interval, err := time.ParseDuration(cfg.RawInterval)
	if err != nil {
		return ClientConfig{}, fmt.Errorf("parse scan_interval: %w", err)
	}
	cfg.ScanInterval = interval
	return cfg, nil
}

func ResolveCodexHome(cfg ClientConfig, env map[string]string, home string) string {
	if cfg.CodexHome != "" {
		return cfg.CodexHome
	}
	if env["CODEX_HOME"] != "" {
		return env["CODEX_HOME"]
	}
	return joinWithSeparator(separatorForPath(home), home, ".codex")
}

func ValidateConfig(cfg ClientConfig) error {
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return fmt.Errorf("server_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("api_key is required")
	}
	if strings.TrimSpace(cfg.IdentityKey) == "" {
		return fmt.Errorf("identity_key is required")
	}
	if cfg.ScanInterval <= 0 {
		return fmt.Errorf("scan_interval must be positive")
	}
	return nil
}

func separatorForPath(path string) string {
	if strings.Contains(path, `\`) && !strings.Contains(path, "/") {
		return `\`
	}
	return "/"
}

func joinWithSeparator(separator string, parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			part = strings.TrimRight(part, `/\`)
		} else {
			part = strings.Trim(part, `/\`)
		}
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, separator)
}
