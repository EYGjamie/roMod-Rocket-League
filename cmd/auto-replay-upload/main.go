// Command auto-replay-upload uploads Rocket League replays to ballchasing.com.
//
// It listens to the game's official Stats API, holds the save-replay key
// once per match so the replay is saved automatically at the end, uploads
// every new file in the replay folder and sorts it into ballchasing groups.
// It runs in the background with an icon in the notification area (the
// menu bar on macOS); the interface opens as an overlay on Windows and in
// the browser.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/app"
	"github.com/EYGjamie/auto-replay-upload/internal/desktop"
	"github.com/EYGjamie/auto-replay-upload/internal/game"
	"github.com/EYGjamie/auto-replay-upload/internal/hotkey"
	"github.com/EYGjamie/auto-replay-upload/internal/overlay"
	"github.com/EYGjamie/auto-replay-upload/internal/popup"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	desktop.SetDPIAware()
	exe, _ := os.Executable()
	defaultCfg := defaultConfig(exe)
	cfgFlag := flag.String("config", defaultCfg, "Pfad zur Konfigurationsdatei")
	flag.Parse()
	cfgPath, err := filepath.Abs(*cfgFlag)
	if err != nil {
		fatal("Konfiguration: %v", err)
	}

	if !desktop.SingleInstance(cfgPath) {
		desktop.ShowInfo(strings.TrimSpace("auto-replay-upload läuft schon. " + desktop.TrayHint))
		return
	}
	logPath := filepath.Join(filepath.Dir(cfgPath), "auto-replay-upload.log")
	if err := openLog(logPath); err != nil {
		fatal("Log-Datei: %v", err)
	}
	log.SetFlags(log.Ltime)
	log.Printf("Gestartet am %s (Version %s)", time.Now().Format("02.01.2006"), version)
	app.Version = version

	hk, err := hotkey.NewManager()
	if err != nil {
		log.Printf("Hotkeys nicht verfügbar: %v", err)
	}
	d := &desk{pop: popup.New(), hotkeys: hk != nil, saveKey: game.SaveKeySupported()}
	a, err := app.New(app.Options{ConfigPath: cfgPath, LogPath: logPath, Desktop: d, Hotkeys: hk})
	if err != nil {
		desktop.Edit(cfgPath)
		fatal("Start fehlgeschlagen: %v", err)
	}
	defer a.Close()
	d.url = a.URL()
	d.overlay = overlay.NewOverlay(d.url)
	d.replayDir = a.ReplayFolder

	d.autostart = []string{exe}
	if cfgPath != defaultCfg {
		d.autostart = append(d.autostart, "-config", cfgPath)
	}
	if strings.Contains(exe, "go-build") {
		// go run builds into a temporary folder that is gone afterwards
		log.Print("Autostart bleibt unverändert (gestartet mit go run)")
	} else if err := desktop.SyncAutostart(d.autostart); err != nil {
		log.Printf("Autostart konnte nicht eingerichtet werden: %v", err)
	} else if desktop.Autostart() {
		log.Printf("Startet %s (in den Einstellungen abschaltbar)", desktop.SignIn)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go a.Run(ctx)

	tray := desktop.Tray{
		Overlay:     func() { d.overlay.Show("") },
		Settings:    func() { d.overlay.Show("einstellungen") },
		TogglePause: a.TogglePause,
		Upload:      func() { a.UploadFiles(d.PickFiles()) },
		OpenFolder:  a.OpenReplayFolder,
	}
	if overlay.Native {
		tray.Browser = func() { overlay.OpenBrowser(d.url) }
	}
	if hk != nil {
		tray.Hotkey = a.Config().GUIHotkey
	}
	tray.Run(ctx)
	stop()
	log.Print("Beendet")
}

// defaultConfig is config.json next to the program on Windows, so the
// whole installation is one folder. Elsewhere programs usually live in a
// folder the user cannot write to, so it is the user's configuration
// folder, unless a config.json lies next to the program.
func defaultConfig(exe string) string {
	beside := filepath.Join(filepath.Dir(exe), "config.json")
	if runtime.GOOS == "windows" {
		return beside
	}
	if _, err := os.Stat(beside); err == nil {
		return beside
	}
	dir, err := os.UserConfigDir() // ~/Library/Application Support, ~/.config
	if err != nil {
		return beside
	}
	return filepath.Join(dir, "auto-replay-upload", "config.json")
}

// desk connects the app to the overlay, popups, tray and the system.
type desk struct {
	url       string
	overlay   *overlay.Overlay
	pop       *popup.Manager
	autostart []string
	replayDir func() string
	hotkeys   bool
	saveKey   bool
}

func (d *desk) ShowOverlay(route string) { d.overlay.Show(route) }
func (d *desk) ToggleOverlay()           { d.overlay.Toggle() }

// OpenBrowser opens a web address, or a view of the interface.
func (d *desk) OpenBrowser(target string) {
	if !strings.HasPrefix(target, "http") {
		target = d.url + "#/" + target
	}
	overlay.OpenBrowser(target)
}

func (d *desk) ShowQuickSort(q app.QuickSort, pick func(int)) {
	d.pop.ShowQuickSort(popup.Quick{Title: q.Title, Target: q.Target, Options: q.Options, Seconds: q.Seconds}, pick)
}

func (d *desk) HideQuickSort() { d.pop.HideQuickSort() }

func (d *desk) Toast(t app.Toast) {
	pt := popup.Toast{Title: t.Title, Body: t.Body, Disable: t.Disable}
	for _, a := range t.Actions {
		pt.Actions = append(pt.Actions, popup.Action{Label: a.Label, Run: a.Run})
	}
	d.pop.Toast(pt)
}

func (d *desk) SetTray(paused, failed bool) { desktop.SetTrayState(paused, failed) }
func (d *desk) Recycle(path string) error   { return desktop.Recycle(path) }
func (d *desk) Reveal(path string) error    { return desktop.Reveal(path) }
func (d *desk) OpenFolder(dir string) error { return desktop.OpenFolder(dir) }
func (d *desk) Edit(path string) error      { return desktop.Edit(path) }
func (d *desk) Autostart() bool             { return desktop.Autostart() }
func (d *desk) SetAutostart(on bool) error  { return desktop.SetAutostart(on, d.autostart) }

// PickFiles starts in the replay folder.
func (d *desk) PickFiles() []string { return desktop.PickFiles(d.replayDir()) }

func (d *desk) Platform() app.Platform {
	return app.Platform{
		OS:        runtime.GOOS,
		Overlay:   overlay.Native,
		Tray:      desktop.HasTray,
		Autostart: desktop.HasAutostart,
		Hotkeys:   d.hotkeys,
		SaveKey:   d.saveKey,
		QuickSort: d.pop.Interactive(),
	}
}

// openLog sends log output to path as well as the terminal, if there is one,
// and the tray tooltip. A log above 1 MB is moved to path.old first.
func openLog(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	// Stderr goes last: without a console its writes fail, which stops the
	// writers after it.
	log.SetOutput(io.MultiWriter(f, desktop.StatusLog, os.Stderr))
	return nil
}

// fatal logs the message, shows it in a message box (there may be no console
// window) and exits.
func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	desktop.ShowError(msg)
	os.Exit(1)
}
