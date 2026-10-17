package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Token      string   `json:"ballchasing_token"`
	Visibility string   `json:"visibility"`
	Group      string   `json:"group"`
	DemoDirs   []string `json:"demo_dirs"`
	StatsAPI   string   `json:"stats_api_addr"`
	AutoMark   bool     `json:"auto_mark"`
	MarkHoldMs int      `json:"mark_hold_ms"`
	OnlineOnly bool     `json:"online_only"`
	GUIHotkey  string   `json:"gui_hotkey"`
}

func defaultConfig() Config {
	return Config{
		Visibility: "private",
		DemoDirs:   []string{},
		StatsAPI:   "127.0.0.1:49123",
		AutoMark:   true,
		MarkHoldMs: 1200,
		OnlineOnly: true,
		GUIHotkey:  "Ctrl+Alt+B",
	}
}

// loadConfig reads path on top of the defaults. BALLCHASING_TOKEN overrides
// the token from the file.
func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if t := os.Getenv("BALLCHASING_TOKEN"); t != "" {
		cfg.Token = t
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch {
	case c.Token == "":
		return errors.New("ballchasing_token fehlt (zu finden unter https://ballchasing.com/upload)")
	case c.Visibility != "public" && c.Visibility != "unlisted" && c.Visibility != "private":
		return fmt.Errorf("visibility %q ungültig, erlaubt: public, unlisted, private", c.Visibility)
	case c.MarkHoldMs < 100:
		return fmt.Errorf("mark_hold_ms %d zu klein (mindestens 100)", c.MarkHoldMs)
	}
	if _, err := parseHotkey(c.GUIHotkey); err != nil {
		return fmt.Errorf("gui_hotkey: %w", err)
	}
	return nil
}

// writeDefaultConfig creates a config file with an empty token for the user
// to fill in.
func writeDefaultConfig(path string) error {
	data, err := json.MarshalIndent(defaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
