// Package desktop runs the tool in the background: an icon in the
// notification area (the menu bar on macOS), autostart at sign-in, a single
// running copy, message boxes and file operations such as the recycle bin.
// Each system gets its own implementation; where something is missing the
// tool still runs and logs instead.
package desktop

import (
	"crypto/sha1"
	"encoding/hex"
	"runtime"
	"strings"
)

// Tray is the notification area icon and its menu.
type Tray struct {
	Overlay     func() // left click and "Overlay öffnen"
	Browser     func() // "Im Browser öffnen"; nil where the overlay is the browser anyway
	Settings    func()
	TogglePause func()
	Upload      func() // pick files and upload them
	OpenFolder  func()
	Hotkey      string // overlay shortcut shown in the menu, may be empty
}

// StatusLog is meant as additional log output: the last line written shows
// up in the icon's tooltip.
var StatusLog statusWriter

var statusCh = make(chan string, 1)

type statusWriter struct{}

func (statusWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line != "" && !strings.Contains(line, "systray") { // the tray library logs its own errors
		select { // keep only the newest line
		case <-statusCh:
		default:
		}
		select {
		case statusCh <- line:
		default:
		}
	}
	return len(p), nil
}

// tooltip fits the name and status line into the 127 characters a tray
// tooltip can hold.
func tooltip(status string) string {
	s := "auto-replay-upload"
	if status != "" {
		s += "\n" + status
	}
	if r := []rune(s); len(r) > 127 {
		s = string(r[:126]) + "…"
	}
	return s
}

// instanceKey names the lock of one configuration. Paths differing only in
// case are the same file on Windows and (by default) on macOS.
func instanceKey(configPath string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		configPath = strings.ToLower(configPath)
	}
	sum := sha1.Sum([]byte(configPath))
	return "auto-replay-upload-" + hex.EncodeToString(sum[:6])
}
