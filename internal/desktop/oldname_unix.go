//go:build darwin || linux

package desktop

import (
	"os"
	"path/filepath"
)

// ForgetOldName carries over the configuration folder and the autostart
// entry of auto-replay-upload, as roMod was called until October 2026.
// SyncAutostart then rewrites the entry under the new name.
func ForgetOldName() {
	if dir, err := os.UserConfigDir(); err == nil {
		moveIfFree(filepath.Join(dir, "auto-replay-upload"), filepath.Join(dir, "romod"))
	}
	if path, err := autostartFile(); err == nil {
		moveIfFree(filepath.Join(filepath.Dir(path), oldAutostartName), path)
	}
}
