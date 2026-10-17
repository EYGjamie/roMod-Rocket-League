//go:build !windows

package overlay

import (
	"os/exec"
	"runtime"
)

// Overlay falls back to the browser; the native window needs Windows.
type Overlay struct{ url string }

func NewOverlay(url string) *Overlay { return &Overlay{url} }

func (o *Overlay) Toggle() { o.Show("") }

func (o *Overlay) Show(route string) {
	url := o.url
	if route != "" {
		url += "#/" + route
	}
	OpenBrowser(url)
}

// OpenBrowser opens a URL in the default browser.
func OpenBrowser(url string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, url).Start()
}
