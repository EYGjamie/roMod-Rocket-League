//go:build darwin

package desktop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Autostart is a launch agent that macOS loads at sign-in. macOS lists it
// under System Settings → General → Login Items.

// oldAutostartName is the entry of auto-replay-upload, see ForgetOldName.
const oldAutostartName = "com.github.eygjamie.auto-replay-upload.plist"

func autostartFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgent+".plist"), nil
}

// Autostart reports whether macOS starts the tool at sign-in.
func Autostart() bool { return entryExists() }

func applyAutostart(on bool, args []string, _ bool) error {
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
	return writeFile(path, agentPlist(args), 0o644)
}
