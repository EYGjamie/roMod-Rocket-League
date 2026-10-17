//go:build windows

package desktop

import (
	"context"
	_ "embed"
	"log"

	"fyne.io/systray"
)

//go:embed icon.ico
var icon []byte

// Run shows the icon until the user picks "Beenden" or ctx is done. It must
// be called from the main goroutine.
func (t Tray) Run(ctx context.Context) {
	systray.Run(func() { t.menu(ctx) }, nil)
}

func (t Tray) menu(ctx context.Context) {
	systray.SetIcon(icon)
	systray.SetTooltip(tooltip(""))
	systray.SetOnTapped(func() { go t.Groups() })

	title := "Replays gruppieren"
	if t.Hotkey != "" {
		title += "\t" + t.Hotkey
	}
	groups := systray.AddMenuItem(title, "")
	systray.AddSeparator()
	autostart := systray.AddMenuItemCheckbox("Mit Windows starten", "", Autostart())
	config := systray.AddMenuItem("Konfiguration bearbeiten", "")
	logs := systray.AddMenuItem("Log anzeigen", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Beenden", "")

	for {
		select {
		case <-groups.ClickedCh:
			t.Groups()
		case <-autostart.ClickedCh:
			on := !Autostart() // may have changed in the Task Manager meanwhile
			if err := SetAutostart(on, t.Autostart); err != nil {
				log.Printf("Autostart konnte nicht geändert werden: %v", err)
			} else if on {
				log.Print("Startet ab jetzt mit Windows")
			} else {
				log.Print("Startet nicht mehr mit Windows")
			}
			if Autostart() {
				autostart.Check()
			} else {
				autostart.Uncheck()
			}
		case <-config.ClickedCh:
			Edit(t.ConfigPath)
		case <-logs.ClickedCh:
			Edit(t.LogPath)
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
