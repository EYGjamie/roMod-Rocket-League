// Command auto-replay-upload uploads Rocket League replays to ballchasing.com.
//
// It listens to the game's official Stats API, holds Backspace (the game's
// "save replay" bind) once per match so the replay is saved automatically at
// the end, and uploads every new file in the replay folder.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	exe, _ := os.Executable()
	cfgPath := flag.String("config", filepath.Join(filepath.Dir(exe), "config.json"), "Pfad zur Konfigurationsdatei")
	flag.Parse()
	log.SetFlags(log.Ltime)

	cfg, err := loadConfig(*cfgPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := writeDefaultConfig(*cfgPath); err != nil {
			fatal("Konfiguration konnte nicht angelegt werden: %v", err)
		}
		fatal("%s angelegt. Trag dort deinen ballchasing-Token ein und starte neu.", *cfgPath)
	}
	if err != nil {
		fatal("Konfiguration: %v", err)
	}

	dirs := cfg.DemoDirs
	if len(dirs) == 0 {
		if dirs, err = defaultDemoDirs(); err != nil {
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

	bc := &ballchasing{
		baseURL:    "https://ballchasing.com",
		token:      cfg.Token,
		visibility: cfg.Visibility,
		group:      cfg.Group,
		client:     &http.Client{Timeout: 2 * time.Minute},
	}
	switch name, err := bc.ping(ctx); {
	case errors.Is(err, errBadToken):
		fatal("Der ballchasing-Token ist ungültig. Neuen Token unter https://ballchasing.com/upload erzeugen.")
	case err != nil:
		log.Printf("ballchasing gerade nicht erreichbar (%v), Uploads werden später versucht", err)
	default:
		log.Printf("Bei ballchasing angemeldet als %s, Sichtbarkeit: %s", name, cfg.Visibility)
	}

	state, existed, err := loadState(filepath.Join(filepath.Dir(*cfgPath), "uploaded.json"))
	if err != nil {
		fatal("Status: %v", err)
	}
	w := newWatcher(dirs, state, bc.upload)
	if !existed {
		n, err := w.seed()
		if err != nil {
			fatal("Status: %v", err)
		}
		log.Printf("Erster Start: %d vorhandene Replays werden nicht hochgeladen", n)
	}
	go w.run(ctx, 3*time.Second)

	guiURL, err := newGUI(bc).start(ctx)
	if err != nil {
		fatal("Oberfläche: %v", err)
	}
	if hk, _ := parseHotkey(cfg.GUIHotkey); hk.vk == 0 {
		log.Printf("Kein Hotkey gesetzt, Gruppen-Seite im Browser: %s", guiURL)
	} else if err := startHotkey(hk, newOverlay(guiURL).toggle); err != nil {
		log.Printf("Hotkey %s nicht verfügbar (%v), Gruppen-Seite im Browser: %s", cfg.GUIHotkey, err, guiURL)
	} else {
		log.Printf("%s öffnet und schließt das Overlay zum Gruppieren", cfg.GUIHotkey)
	}

	t := &tracker{
		onlineOnly: cfg.OnlineOnly,
		autoMark:   cfg.AutoMark,
		delay:      func() time.Duration { return 3*time.Second + rand.N(5*time.Second) },
		retry:      3 * time.Second,
		mark: func(ctx context.Context) error {
			return markReplay(ctx, time.Duration(cfg.MarkHoldMs)*time.Millisecond)
		},
		remind: beep,
	}
	if cfg.AutoMark {
		log.Print("Replays werden nach dem ersten Anstoß automatisch vorgemerkt")
	} else {
		log.Print("Automatisches Vormerken ist aus, am Matchende kommt ein Erinnerungston")
	}
	listenStatsAPI(ctx, cfg.StatsAPI, t.handle)
}

// fatal logs the message and waits for Enter, so the console window stays
// open when the exe was started by double-click.
func fatal(format string, args ...any) {
	log.Printf(format, args...)
	fmt.Print("Enter drücken zum Beenden …")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}
