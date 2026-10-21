//go:build windows || linux || (darwin && cgo)

package desktop

import (
	"context"
	"log"
	"sync"

	"fyne.io/systray"

	"github.com/EYGjamie/roMod-Rocket-League/internal/assets"
)

// HasTray reports that the tool shows an icon.
const HasTray = true

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
		systray.SetIcon(assets.TrayError)
	case trayState.paused:
		systray.SetIcon(assets.TrayPaused)
	default:
		systray.SetIcon(assets.Tray)
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
	if err := trayHost(); err != nil {
		log.Printf("Kein Symbol im Infobereich: %v", err)
		<-ctx.Done()
		return
	}
	systray.Run(func() {
		// The ready callback has to return: on Linux the panel gets the
		// menu only after that.
		loop := t.menu(ctx)
		go loop()
	}, nil)
}

// menu builds the icon and menu and returns the loop that handles clicks.
func (t Tray) menu(ctx context.Context) func() {
	systray.SetIcon(assets.Tray)
	systray.SetTooltip(tooltip(""))
	if leftClickOpens {
		systray.SetOnTapped(func() { go t.Overlay() })
	}

	pause := systray.AddMenuItem("Pausieren", "")
	upload := systray.AddMenuItem("Replays hochladen …", "")
	folder := systray.AddMenuItem("Replay-Ordner öffnen", "")
	systray.AddSeparator()
	title := "Overlay öffnen"
	if t.Browser == nil {
		title = "Oberfläche öffnen"
	}
	overlay := systray.AddMenuItem(menuTitle(title, t.Hotkey), "")
	var browser <-chan struct{} // stays nil without the entry
	if t.Browser != nil {
		browser = systray.AddMenuItem("Im Browser öffnen", "").ClickedCh
	}
	settings := systray.AddMenuItem("Einstellungen", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Beenden", "")

	trayState.Lock()
	trayState.pause, trayState.ready = pause, true
	applyTrayState()
	trayState.Unlock()

	return func() {
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
			case <-browser:
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
}
