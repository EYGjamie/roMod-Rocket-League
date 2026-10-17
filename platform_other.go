//go:build !windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

// Marking replays, the hotkey and the overlay need Windows APIs; elsewhere
// uploading and the GUI (via the printed link) still work.

func markReplay(context.Context, time.Duration) error {
	return errors.New("Tastendruck wird nur unter Windows unterstützt")
}

func beep() {}

func defaultDemoDirs() ([]string, error) {
	return nil, errors.New("kein Standardordner außerhalb von Windows, bitte demo_dirs setzen")
}

func startHotkey(hotkey, func()) error {
	return errors.New("Hotkeys werden nur unter Windows unterstützt")
}

// overlay falls back to the browser; there is no native window here.
type overlay struct{ url string }

func newOverlay(url string) *overlay { return &overlay{url} }

func (o *overlay) toggle() { openBrowser(o.url) }

func openBrowser(url string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, url).Start()
}
