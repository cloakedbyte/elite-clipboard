package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type WorkspaceConfig struct {
	Name          string `json:"name"`
	MaxItems      int    `json:"max_items"`
	RetentionDays int    `json:"retention_days"`
}

type Config struct {
	MaxItems         int                        `json:"max_items"`
	SizeCapMB        int                        `json:"size_cap_mb"`
	PollIntervalMS   int                        `json:"poll_interval_ms"`
	MonitorClipboard bool                       `json:"monitor_clipboard"`
	MonitorPrimary   bool                       `json:"monitor_primary"`
	MinContentLength int                        `json:"min_content_length"`
	ExcludedApps     []string                   `json:"excluded_apps"`
	Workspaces       map[string]WorkspaceConfig `json:"workspaces"`
}

func Default() *Config {
	return &Config{
		MaxItems:         500,
		SizeCapMB:        100,
		PollIntervalMS:   1000,
		MonitorClipboard: true,
		MonitorPrimary:   false,
		MinContentLength: 4,
		ExcludedApps:     []string{"keepassxc", "KeePassXC", "gnome-keyring"},
		Workspaces: map[string]WorkspaceConfig{
			"0": {Name: "Work",      MaxItems: 200, RetentionDays: 30},
			"1": {Name: "Coding",    MaxItems: 300, RetentionDays: 60},
			"2": {Name: "Research",  MaxItems: 200, RetentionDays: 30},
			"3": {Name: "Temporary", MaxItems: 50,  RetentionDays: 1},
			"4": {Name: "Sensitive", MaxItems: 50,  RetentionDays: 1},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, Save(cfg, path)
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), nil
	}
	return cfg, nil
}

func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
