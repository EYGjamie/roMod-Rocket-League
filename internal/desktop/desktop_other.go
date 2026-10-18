//go:build !windows && !darwin && !linux

package desktop

import (
	"errors"
	"os/exec"
	"path/filepath"
)

// On other systems the tool runs in the terminal: messages are already in
// the log there, and autostart is left to the user.

const (
	TrayHint     = ""
	SignIn       = "bei der Anmeldung"
	HasAutostart = false
)

func Autostart() bool { return false }

func SyncAutostart([]string) error { return nil }

func SetAutostart(bool, []string) error {
	return errors.New("Autostart wird auf diesem System nicht unterstützt")
}

func SetDPIAware() {}

func SingleInstance(string) bool { return true }

func ShowError(string) {}

func ShowInfo(string) {}

func Edit(path string) error { return exec.Command("xdg-open", path).Start() }

// Recycle refuses: without a known recycle bin the file stays where it is.
func Recycle(string) error { return errors.New("kein Papierkorb auf diesem System") }

func Reveal(path string) error { return OpenFolder(filepath.Dir(path)) }

func OpenFolder(dir string) error { return Edit(dir) }

func PickFiles(string) []string { return nil }
