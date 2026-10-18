//go:build linux

package desktop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Autostart uses the freedesktop.org autostart folder, which GNOME, KDE,
// Xfce and the other desktops read at sign-in.

func autostartFile() (string, error) {
	dir, err := os.UserConfigDir() // $XDG_CONFIG_HOME or ~/.config
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "auto-replay-upload.desktop"), nil
}

// Autostart reports whether the desktop starts the tool at sign-in.
func Autostart() bool {
	path, err := autostartFile()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && !entrySwitchedOff(string(data))
}

func applyAutostart(on bool, args []string, keepOff bool) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	off := false
	if data, err := os.ReadFile(path); err == nil && keepOff {
		off = entrySwitchedOff(string(data))
	}
	return writeFile(path, []byte(desktopEntry(args, off)), 0o644)
}
