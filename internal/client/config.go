package client

import (
	"fmt"
	"os"
	"strings"
	"time"
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
	values, err := loadSimpleYAML(path)
	if err != nil {
		return ClientConfig{}, err
	}

	cfg := ClientConfig{
		ServerURL:   values["server_url"],
		APIKey:      values["api_key"],
		IdentityKey: values["identity_key"],
		RawInterval: values["scan_interval"],
		CodexHome:   values["codex_home"],
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

func loadSimpleYAML(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("parse yaml line %d: expected key: value", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("parse yaml line %d: empty key", lineNumber+1)
		}
		values[key] = unquoteYAMLScalar(value)
	}
	return values, nil
}

func unquoteYAMLScalar(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
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
