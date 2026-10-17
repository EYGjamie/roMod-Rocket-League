package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveEncryptsTokenAndLoadRestoresIt(t *testing.T) {
	t.Setenv("BALLCHASING_TOKEN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, err := Load(path)
	if !errors.Is(err, fs.ErrNotExist) || cfg.QuickSortSeconds != 10 || cfg.MarkKey != "Backspace" {
		t.Fatalf("Load(missing) = %+v, %v", cfg, err)
	}
	cfg.SetToken("geheim-123")
	cfg.Visibility = "public"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "geheim-123") || !strings.Contains(string(data), "ballchasing_token_protected") {
		t.Errorf("token stored in plain text:\n%s", data)
	}
	got, err := Load(path)
	if err != nil || got.Token != "geheim-123" || got.Visibility != "public" {
		t.Errorf("Load() = %+v, %v", got, err)
	}
}

func TestPlainTokenFromOldVersionsIsEncryptedOnSave(t *testing.T) {
	t.Setenv("BALLCHASING_TOKEN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"ballchasing_token":"alt","visibility":"private","group":"x"}`), 0o600)
	cfg, err := Load(path)
	if err != nil || cfg.Token != "alt" || !cfg.AutoMark {
		t.Fatalf("Load(old file) = %+v, %v", cfg, err)
	}
	Save(path, cfg)
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"alt"`) || strings.Contains(string(data), `"group"`) {
		t.Errorf("old fields survived:\n%s", data)
	}
	if got, _ := Load(path); got.Token != "alt" {
		t.Errorf("token lost: %q", got.Token)
	}
}

func TestEnvTokenIsNotSaved(t *testing.T) {
	t.Setenv("BALLCHASING_TOKEN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.SetToken("datei")
	Save(path, cfg)
	t.Setenv("BALLCHASING_TOKEN", "umgebung")
	cfg, _ = Load(path)
	if cfg.Token != "umgebung" || !cfg.TokenFromEnv() {
		t.Fatalf("token = %q, want the environment's", cfg.Token)
	}
	Save(path, cfg)
	t.Setenv("BALLCHASING_TOKEN", "")
	if got, _ := Load(path); got.Token != "datei" {
		t.Errorf("saved token = %q, want the file's", got.Token)
	}
}

func TestValidate(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"visibility":   func(c *Config) { c.Visibility = "geheim" },
		"hold":         func(c *Config) { c.MarkHoldMs = 50 },
		"key":          func(c *Config) { c.MarkKey = "Hyper" },
		"countdown":    func(c *Config) { c.QuickSortSeconds = 1 },
		"hotkey":       func(c *Config) { c.PauseHotkey = "Ctrl+Nix" },
		"same hotkeys": func(c *Config) { c.PauseHotkey = c.GUIHotkey },
	} {
		c := Default()
		mutate(&c)
		if c.Validate() == nil {
			t.Errorf("%s: Validate() accepted %+v", name, c)
		}
	}
	if err := Default().Validate(); err != nil {
		t.Errorf("defaults invalid: %v", err)
	}
}

func TestLoadAcceptsBOM(t *testing.T) {
	t.Setenv("BALLCHASING_TOKEN", "")
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, append([]byte("\xef\xbb\xbf"), []byte(`{"visibility":"public"}`)...), 0o600)
	if cfg, err := Load(path); err != nil || cfg.Visibility != "public" {
		t.Errorf("Load(with BOM) = %q, %v", cfg.Visibility, err)
	}
}
