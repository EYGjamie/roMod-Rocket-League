//go:build !windows

package overlay

import (
	"os/exec"
	"runtime"
)

// Native is false: the overlay needs WebView2, so the interface opens in
// the browser instead.
const Native = false

// Overlay opens the interface in the browser.
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
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, url)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
