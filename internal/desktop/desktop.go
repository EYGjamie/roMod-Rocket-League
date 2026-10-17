// Package desktop runs the tool in the background on Windows: notification
// area icon, autostart at sign-in, a single running copy, message boxes and
// file operations. Elsewhere most of it does nothing and the tool runs in
// the terminal.
package desktop

import "strings"

// Tray is the notification area icon and its menu.
type Tray struct {
	Overlay     func() // left click and "Overlay öffnen"
	Browser     func()
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
