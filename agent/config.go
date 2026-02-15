package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configDir = "/etc/linux-user-manager"
const configPath = "/etc/linux-user-manager/config.json"

type Config struct {
	ComputerID int    `json:"computer_id"`
	ServerURL  string `json:"server_url"`
	ApiKey     string `json:"api_key"`
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config file: %w", err)
	}

	if cfg.ComputerID == 0 || cfg.ServerURL == "" {
		return nil, fmt.Errorf("config missing computer_id or server_url")
	}

	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
