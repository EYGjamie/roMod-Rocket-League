//go:build !windows

package gui

import (
	"os/exec"
	"runtime"
)

// Overlay falls back to the browser; the native window needs Windows.
type Overlay struct{ url string }

func NewOverlay(url string) *Overlay { return &Overlay{url} }

func (o *Overlay) Toggle() { openBrowser(o.url) }

func openBrowser(url string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, url).Start()
}
