//go:build darwin || linux

package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// HasAutostart reports that starting at sign-in can be switched.
const HasAutostart = true

// The choice from the settings is remembered apart from the autostart
// entry: "on", "off", or nothing before the first start.
func choiceFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "romod", "autostart"), nil
}

func autostartChoice() string {
	path, err := choiceFile()
	if err != nil {
		return ""
	}
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func rememberAutostart(on bool) error {
	path, err := choiceFile()
	if err != nil {
		return err
	}
	v := "on\n"
	if !on {
		v = "off\n"
	}
	return writeFile(path, []byte(v), 0o644)
}

// SyncAutostart turns autostart on at the first start and afterwards keeps
// the path in args current in case the program moved. An entry the user
// removed or switched off in the system settings stays that way.
func SyncAutostart(args []string) error {
	switch autostartChoice() {
	case "off":
		return applyAutostart(false, args, true)
	case "on":
		if !entryExists() {
			return rememberAutostart(false)
		}
		return applyAutostart(true, args, true)
	default:
		if err := applyAutostart(true, args, true); err != nil {
			return err
		}
		return rememberAutostart(true)
	}
}

// SetAutostart remembers the choice and makes the system run args at
// sign-in, or stops that.
func SetAutostart(on bool, args []string) error {
	if err := applyAutostart(on, args, false); err != nil {
		return err
	}
	return rememberAutostart(on)
}

func entryExists() bool {
	path, err := autostartFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// writeFile replaces path atomically, and not at all if nothing changed.
func writeFile(path string, data []byte, perm os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
