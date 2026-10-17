//go:build windows

package desktop

import (
	"context"
	_ "embed"
	"sync"

	"fyne.io/systray"
)

var (
	//go:embed icon.ico
	icon []byte
	//go:embed icon-paused.ico
	iconPaused []byte
	//go:embed icon-error.ico
	iconError []byte
)

// trayState is applied to the icon and menu once they exist.
var trayState struct {
	sync.Mutex
	paused, failed bool
	ready          bool
	pause          *systray.MenuItem
}

// SetTrayState shows whether everything is paused and whether uploads
// failed, via the icon and the pause entry.
func SetTrayState(paused, failed bool) {
	trayState.Lock()
	defer trayState.Unlock()
	trayState.paused, trayState.failed = paused, failed
	if trayState.ready {
		applyTrayState()
	}
}

func applyTrayState() {
	switch {
	case trayState.failed:
		systray.SetIcon(iconError)
	case trayState.paused:
		systray.SetIcon(iconPaused)
	default:
		systray.SetIcon(icon)
	}
	if trayState.paused {
		trayState.pause.SetTitle("Fortsetzen")
	} else {
		trayState.pause.SetTitle("Pausieren")
	}
}

// Run shows the icon until the user picks "Beenden" or ctx is done. It must
// be called from the main goroutine.
func (t Tray) Run(ctx context.Context) {
	systray.Run(func() { t.menu(ctx) }, nil)
}

func (t Tray) menu(ctx context.Context) {
	systray.SetIcon(icon)
	systray.SetTooltip(tooltip(""))
	systray.SetOnTapped(func() { go t.Overlay() })

	pause := systray.AddMenuItem("Pausieren", "")
	upload := systray.AddMenuItem("Replays hochladen …", "")
	folder := systray.AddMenuItem("Replay-Ordner öffnen", "")
	systray.AddSeparator()
	title := "Overlay öffnen"
	if t.Hotkey != "" {
		title += "\t" + t.Hotkey
	}
	overlay := systray.AddMenuItem(title, "")
	browser := systray.AddMenuItem("Im Browser öffnen", "")
	settings := systray.AddMenuItem("Einstellungen", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Beenden", "")

	trayState.Lock()
	trayState.pause, trayState.ready = pause, true
	applyTrayState()
	trayState.Unlock()

	for {
		select {
		case <-pause.ClickedCh:
			go t.TogglePause()
		case <-upload.ClickedCh:
			go t.Upload()
		case <-folder.ClickedCh:
			go t.OpenFolder()
		case <-overlay.ClickedCh:
			go t.Overlay()
		case <-browser.ClickedCh:
			go t.Browser()
		case <-settings.ClickedCh:
			go t.Settings()
		case s := <-statusCh:
			systray.SetTooltip(tooltip(s))
		case <-quit.ClickedCh:
			systray.Quit()
			return
		case <-ctx.Done():
			systray.Quit()
			return
		}
	}
}
