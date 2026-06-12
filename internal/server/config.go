package server

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	ListenAddr    string `yaml:"listen_addr"`
	PublicBaseURL string `yaml:"public_base_url"`
	APIKey        string `yaml:"api_key"`
	SQLitePath    string `yaml:"sqlite_path"`
}

func LoadConfig(path string) (ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ServerConfig{}, err
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}

func ValidateConfig(cfg *ServerConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("api_key is required")
	}
	if strings.TrimSpace(cfg.SQLitePath) == "" {
		return fmt.Errorf("sqlite_path is required")
	}
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = ":8080"
	}
	return nil
}
