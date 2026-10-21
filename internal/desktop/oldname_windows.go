//go:build windows

package desktop

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// ForgetOldName carries over what auto-replay-upload, as roMod was called
// until October 2026, left in the registry and the cache folder. The
// autostart entry itself is written anew by SyncAutostart.
func ForgetOldName() {
	const old = "auto-replay-upload"
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\`+old, registry.QUERY_VALUE); err == nil {
		v, _, err := k.GetIntegerValue(settingsValue)
		k.Close()
		if n, _, cerr := registry.CreateKey(registry.CURRENT_USER, settingsKey, registry.SET_VALUE); err == nil && cerr == nil {
			n.SetDWordValue(settingsValue, uint32(v))
			n.Close()
		}
		registry.DeleteKey(registry.CURRENT_USER, `Software\`+old)
	}
	// switched off in the Task Manager stays switched off
	if k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if b, _, err := k.GetBinaryValue(old); err == nil {
			k.SetBinaryValue(runValue, b)
			k.DeleteValue(old)
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue(old)
		k.Close()
	}
	if dir, err := os.UserCacheDir(); err == nil {
		moveIfFree(filepath.Join(dir, old), filepath.Join(dir, "romod"))
	}
}
