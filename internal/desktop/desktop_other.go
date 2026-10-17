//go:build !windows

package desktop

import (
	"context"
	"errors"
	"os/exec"
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

func SingleInstance() bool { return true }

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
