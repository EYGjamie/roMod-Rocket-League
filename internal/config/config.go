// Package config reads and writes config.json. The app edits it itself;
// the ballchasing token is stored encrypted for the Windows user.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
	"github.com/EYGjamie/roMod-Rocket-League/internal/hotkey"
)

type Config struct {
	Token          string `json:"ballchasing_token,omitempty"` // only read; written encrypted below
	TokenProtected string `json:"ballchasing_token_protected,omitempty"`
	Visibility     string `json:"visibility"`

	DemoDirs       []string `json:"demo_dirs"`
	UploadExisting bool     `json:"upload_existing"`     // upload old, never uploaded replays at start
	DeleteUploaded bool     `json:"delete_after_upload"` // move replays to the recycle bin after upload

	StatsAPI     string `json:"stats_api_addr"`
	StatsAPIAuto bool   `json:"stats_api_auto"` // switch the Stats API on in the game's ini at start
	AutoMark     bool   `json:"auto_mark"`
	OnlineOnly   bool   `json:"online_only"`
	MarkKey      string `json:"mark_key"` // KeyboardEvent.code of the game's save replay key
	MarkHoldMs   int    `json:"mark_hold_ms"`

	GUIHotkey        string    `json:"gui_hotkey"`
	PauseHotkey      string    `json:"pause_hotkey"`
	QuickSortHotkeys [4]string `json:"quick_sort_hotkeys"` // alternatives 1–3 and the inbox while the prompt shows

	NotifyUploaded   bool `json:"notify_uploaded"`
	NotifyFailed     bool `json:"notify_failed"`
	NotifySession    bool `json:"notify_session"`
	QuickSort        bool `json:"quick_sort"`
	QuickSortSeconds int  `json:"quick_sort_seconds"`

	StartMinimized bool `json:"start_minimized"`
	SetupDone      bool `json:"setup_done"`

	// Developer mode, unlocked with a password, and what it offers: ranks
	// and wins of the lobby through a second Epic account.
	DevMode            bool   `json:"dev_mode"`
	MMR                bool   `json:"mmr"`           // look up the lobby at match start
	LobbyPopup         bool   `json:"lobby_popup"`   // show it at match start
	LobbySeconds       int    `json:"lobby_seconds"` // until the popup closes again
	LobbyHotkey        string `json:"lobby_hotkey"`
	SmurfHotkey        string `json:"smurf_hotkey"`
	EpicToken          string `json:"-"` // refresh token of the Epic account; written encrypted below
	EpicTokenProtected string `json:"epic_token_protected,omitempty"`
	EpicName           string `json:"epic_name,omitempty"`

	storedToken  string // token from the file, kept when the environment overrides it
	tokenFromEnv bool
}

// Default returns the settings of a fresh installation.
func Default() Config {
	return Config{
		Visibility:       "private",
		DemoDirs:         []string{},
		StatsAPI:         "127.0.0.1:49123",
		AutoMark:         true,
		OnlineOnly:       true,
		MarkKey:          "Backspace",
		MarkHoldMs:       1200,
		GUIHotkey:        "Ctrl+Alt+B",
		PauseHotkey:      "Ctrl+Alt+P",
		QuickSortHotkeys: [4]string{"Ctrl+Alt+1", "Ctrl+Alt+2", "Ctrl+Alt+3", "Ctrl+Alt+4"},
		NotifyUploaded:   true,
		NotifyFailed:     true,
		NotifySession:    true,
		QuickSort:        true,
		QuickSortSeconds: 10,
		StartMinimized:   true,
		LobbyPopup:       true,
		LobbySeconds:     15,
		LobbyHotkey:      "Ctrl+Alt+L",
		SmurfHotkey:      "Ctrl+Alt+S",
	}
}

// LobbyOn reports whether the lobby lookup runs: developer mode and the
// option are on.
func (c Config) LobbyOn() bool { return c.DevMode && c.MMR }

// Load reads path on top of the defaults. A missing file returns the
// defaults with an error matching fs.ErrNotExist. BALLCHASING_TOKEN
// overrides the token from the file.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		cfg.applyEnv()
		return cfg, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // Notepad and PowerShell may write a BOM
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Normalize()
	if cfg.TokenProtected != "" {
		if t, err := unprotect(cfg.TokenProtected); err == nil {
			cfg.Token = t
		} else {
			cfg.Token = "" // e.g. copied from another Windows user
		}
	}
	if cfg.EpicTokenProtected != "" {
		cfg.EpicToken, _ = unprotect(cfg.EpicTokenProtected) // "" if copied from another Windows user
	}
	cfg.storedToken = cfg.Token
	cfg.applyEnv()
	return cfg, cfg.Validate()
}

// Normalize cleans what a user typed: ~ at the start of a replay folder
// stands for the home folder, as in a shell.
func (c *Config) Normalize() {
	dirs := make([]string, 0, len(c.DemoDirs))
	for _, d := range c.DemoDirs {
		d = strings.TrimSpace(d)
		if d == "~" || strings.HasPrefix(d, "~/") || strings.HasPrefix(d, `~\`) {
			if home, err := os.UserHomeDir(); err == nil {
				d = filepath.Join(home, d[1:])
			}
		}
		dirs = append(dirs, d)
	}
	c.DemoDirs = dirs
}

func (c *Config) applyEnv() {
	if t := os.Getenv("BALLCHASING_TOKEN"); t != "" {
		c.Token, c.tokenFromEnv = t, true
	}
}

// TokenFromEnv reports whether the token comes from BALLCHASING_TOKEN.
func (c Config) TokenFromEnv() bool { return c.tokenFromEnv }

// SetToken changes the token that is saved; a token from the environment
// still takes precedence while it is set.
func (c *Config) SetToken(t string) {
	c.storedToken = t
	if !c.tokenFromEnv {
		c.Token = t
	}
}

// Validate checks values a user could have mistyped in the file or the
// settings page.
func (c Config) Validate() error {
	switch {
	case c.Visibility != "public" && c.Visibility != "unlisted" && c.Visibility != "private":
		return fmt.Errorf("visibility %q ungültig, erlaubt: public, unlisted, private", c.Visibility)
	case c.MarkHoldMs < 100 || c.MarkHoldMs > 5000:
		return fmt.Errorf("mark_hold_ms %d ungültig (100 bis 5000)", c.MarkHoldMs)
	case !game.ValidKey(c.MarkKey):
		return fmt.Errorf("mark_key %q ist keine unterstützte Taste", c.MarkKey)
	case c.QuickSortSeconds < 3 || c.QuickSortSeconds > 60:
		return fmt.Errorf("quick_sort_seconds %d ungültig (3 bis 60)", c.QuickSortSeconds)
	case c.LobbySeconds < 3 || c.LobbySeconds > 120:
		return fmt.Errorf("lobby_seconds %d ungültig (3 bis 120)", c.LobbySeconds)
	}
	keys := []string{c.GUIHotkey, c.PauseHotkey}
	keys = append(keys, c.QuickSortHotkeys[:]...)
	names := hotkeyNames
	if c.LobbyOn() { // the others are not registered, so they block nothing
		keys = append(keys, c.LobbyHotkey, c.SmurfHotkey)
	} else {
		names = names[:len(keys)]
	}
	used := map[hotkey.Hotkey]bool{}
	for i, key := range keys {
		hk, err := hotkey.Parse(key)
		if err != nil {
			return fmt.Errorf("%s: %w", names[i], err)
		}
		if hk.IsZero() {
			continue
		}
		if used[hk] {
			return fmt.Errorf("%s ist schon vergeben, jeder Hotkey braucht eine eigene Kombination", hotkey.Label(key))
		}
		used[hk] = true
	}
	return nil
}

// hotkeyNames name the hotkeys in the order Validate checks them.
var hotkeyNames = []string{"gui_hotkey", "pause_hotkey", "quick_sort_hotkeys[0]", "quick_sort_hotkeys[1]", "quick_sort_hotkeys[2]", "quick_sort_hotkeys[3]", "lobby_hotkey", "smurf_hotkey"}

// Save writes c to path, atomically and with the tokens encrypted.
func Save(path string, c Config) error {
	c.Token, c.TokenProtected, c.EpicTokenProtected = "", "", ""
	if c.storedToken != "" {
		p, err := protect(c.storedToken)
		if err != nil {
			return err
		}
		c.TokenProtected = p
	}
	if c.EpicToken != "" {
		p, err := protect(c.EpicToken)
		if err != nil {
			return err
		}
		c.EpicTokenProtected = p
	}
	if c.DemoDirs == nil {
		c.DemoDirs = []string{}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
