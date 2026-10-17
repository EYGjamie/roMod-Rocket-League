//go:build !windows

package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Run waits for ctx; there is no notification area icon here.
func (t Tray) Run(ctx context.Context) { <-ctx.Done() }

func Autostart() bool { return false }

func SyncAutostart([]string) error { return nil }

func SetAutostart(bool, []string) error {
	return errors.New("Autostart wird nur unter Windows unterstützt")
}

func SetDPIAware() {}

func SingleInstance(string) bool { return true }

// Messages are already in the log, which goes to the terminal here.

func ShowError(string) {}

func ShowInfo(string) {}

func Edit(path string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, path).Start()
}

func SetTrayState(paused, failed bool) {}

// Recycle deletes the file; there is no recycle bin to rely on here.
func Recycle(path string) error { return os.Remove(path) }

func Reveal(path string) error { return OpenFolder(filepath.Dir(path)) }

func OpenFolder(dir string) error { return Edit(dir) }

func PickFiles(dir string) []string { return nil }
