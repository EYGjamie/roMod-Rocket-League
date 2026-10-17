// Command auto-replay-upload uploads Rocket League replays to ballchasing.com.
//
// It listens to the game's official Stats API, holds Backspace (the game's
// "save replay" bind) once per match so the replay is saved automatically at
// the end, and uploads every new file in the replay folder. On Windows it
// runs in the background with an icon in the notification area.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/config"
	"github.com/EYGjamie/auto-replay-upload/internal/desktop"
	"github.com/EYGjamie/auto-replay-upload/internal/game"
	"github.com/EYGjamie/auto-replay-upload/internal/gui"
	"github.com/EYGjamie/auto-replay-upload/internal/hotkey"
	"github.com/EYGjamie/auto-replay-upload/internal/upload"
)

func main() {
	desktop.SetDPIAware()
	exe, _ := os.Executable()
	defaultCfg := filepath.Join(filepath.Dir(exe), "config.json")
	cfgPath := flag.String("config", defaultCfg, "Pfad zur Konfigurationsdatei")
	flag.Parse()

	if !desktop.SingleInstance() {
		desktop.ShowInfo("auto-replay-upload läuft schon. Das Symbol findest du im Infobereich der Taskleiste, eventuell unter den ausgeblendeten Symbolen.")
		return
	}
	logPath := filepath.Join(filepath.Dir(*cfgPath), "auto-replay-upload.log")
	if err := openLog(logPath); err != nil {
		fatal("Log-Datei: %v", err)
	}
	log.SetFlags(log.Ltime)
	log.Printf("Gestartet am %s", time.Now().Format("02.01.2006"))

	cfg, err := config.Load(*cfgPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := config.WriteDefault(*cfgPath); err != nil {
			fatal("Konfiguration konnte nicht angelegt werden: %v", err)
		}
		msg := *cfgPath + " angelegt. Trag dort deinen ballchasing-Token ein und starte das Programm neu."
		log.Print(msg)
		desktop.ShowInfo(msg)
		desktop.Edit(*cfgPath)
		return
	}
	if err != nil {
		desktop.Edit(*cfgPath)
		fatal("Konfiguration: %v", err)
	}

	dirs := cfg.DemoDirs
	if len(dirs) == 0 {
		if dirs, err = game.DefaultDemoDirs(); err != nil {
			fatal("Replay-Ordner: %v", err)
		}
	}
	watching := false
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			log.Printf("Beobachte %s", d)
			watching = true
		}
	}
	if !watching {
		log.Printf("Noch kein Replay-Ordner vorhanden, warte auf: %s", strings.Join(dirs, ", "))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bc := &ballchasing.Client{
		BaseURL:    "https://ballchasing.com",
		Token:      cfg.Token,
		Visibility: cfg.Visibility,
		Group:      cfg.Group,
		HTTPClient: &http.Client{Timeout: 2 * time.Minute},
	}
	switch name, err := bc.Ping(ctx); {
	case errors.Is(err, ballchasing.ErrBadToken):
		desktop.Edit(*cfgPath)
		fatal("Der ballchasing-Token ist ungültig. Neuen Token unter https://ballchasing.com/upload erzeugen und in die Konfiguration eintragen.")
	case err != nil:
		log.Printf("ballchasing gerade nicht erreichbar (%v), Uploads werden später versucht", err)
	default:
		log.Printf("Bei ballchasing angemeldet als %s, Sichtbarkeit: %s", name, cfg.Visibility)
	}

	state, existed, err := upload.LoadState(filepath.Join(filepath.Dir(*cfgPath), "uploaded.json"))
	if err != nil {
		fatal("Status: %v", err)
	}
	w := upload.NewWatcher(dirs, state, bc.Upload)
	if !existed {
		n, err := w.Seed()
		if err != nil {
			fatal("Status: %v", err)
		}
		log.Printf("Erster Start: %d vorhandene Replays werden nicht hochgeladen", n)
	}
	go w.Run(ctx, 3*time.Second)

	guiURL, err := gui.NewServer(bc).Start(ctx)
	if err != nil {
		fatal("Oberfläche: %v", err)
	}
	overlay := gui.NewOverlay(guiURL)
	hk, _ := hotkey.Parse(cfg.GUIHotkey)
	if hk == (hotkey.Hotkey{}) {
		log.Printf("Kein Hotkey gesetzt, Gruppen-Seite im Browser: %s", guiURL)
	} else if err := hotkey.Start(hk, overlay.Toggle); err != nil {
		log.Printf("Hotkey %s nicht verfügbar (%v), Gruppen-Seite im Browser: %s", cfg.GUIHotkey, err, guiURL)
		hk = hotkey.Hotkey{}
	} else {
		log.Printf("%s öffnet und schließt das Overlay zum Gruppieren", cfg.GUIHotkey)
	}

	t := game.NewTracker(cfg.OnlineOnly, cfg.AutoMark, time.Duration(cfg.MarkHoldMs)*time.Millisecond)
	if cfg.AutoMark {
		log.Print("Replays werden nach dem ersten Anstoß automatisch vorgemerkt")
	} else {
		log.Print("Automatisches Vormerken ist aus, am Matchende kommt ein Erinnerungston")
	}
	go game.ListenStatsAPI(ctx, cfg.StatsAPI, t.Handle)

	autostart := []string{exe}
	if *cfgPath != defaultCfg {
		abs, _ := filepath.Abs(*cfgPath)
		autostart = append(autostart, "-config", abs)
	}
	if strings.Contains(exe, "go-build") {
		// go run builds into a temporary folder that is gone afterwards
		log.Print("Autostart bleibt unverändert (gestartet mit go run)")
	} else if err := desktop.SyncAutostart(autostart); err != nil {
		log.Printf("Autostart konnte nicht eingerichtet werden: %v", err)
	} else if desktop.Autostart() {
		log.Print("Startet mit Windows (im Symbol-Menü abschaltbar)")
	}

	tray := desktop.Tray{
		Groups:     overlay.Toggle,
		ConfigPath: *cfgPath,
		LogPath:    logPath,
		Autostart:  autostart,
	}
	if hk != (hotkey.Hotkey{}) {
		tray.Hotkey = cfg.GUIHotkey
	}
	tray.Run(ctx)
	log.Print("Beendet")
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
