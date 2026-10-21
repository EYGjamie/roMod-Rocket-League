// Command romod (roMod, Replay operations) uploads Rocket League replays to ballchasing.com.
//
// It listens to the game's official Stats API, holds the save-replay key
// once per match so the replay is saved automatically at the end, uploads
// every new file in the replay folder and sorts it into ballchasing groups.
// It runs in the background with an icon in the notification area (the
// menu bar on macOS); the interface opens as an overlay on Windows and in
// the browser.
package main

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64,arm64

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/app"
	"github.com/EYGjamie/roMod-Rocket-League/internal/desktop"
	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
	"github.com/EYGjamie/roMod-Rocket-League/internal/hotkey"
	"github.com/EYGjamie/roMod-Rocket-League/internal/overlay"
	"github.com/EYGjamie/roMod-Rocket-League/internal/popup"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

// pathList is a flag that may be given several times.
type pathList []string

func (p *pathList) String() string     { return strings.Join(*p, ", ") }
func (p *pathList) Set(v string) error { *p = append(*p, v); return nil }

func main() {
	desktop.HideConsole()
	desktop.SetDPIAware()
	exe, _ := os.Executable()
	defaultCfg := defaultConfig(exe)
	cfgFlag := flag.String("config", defaultCfg, "Pfad zur Konfigurationsdatei")
	atSignIn := flag.Bool("autostart", false, "bei der Anmeldung gestartet: die Oberfläche öffnet sich nur ohne start_minimized")
	var statsINIs pathList
	flag.Var(&statsINIs, "enable-stats-api", "schaltet in dieser ini des Spiels die Stats API ein und beendet sich (die Oberfläche ruft das mit Administratorrechten auf)")
	statsPort := flag.Int("stats-api-port", game.DefaultStatsPort, "Port für -enable-stats-api")
	updated := flag.Bool("updated", false, "nach einem Update gestartet: wartet, bis die alte Version beendet ist")
	flag.Parse()
	if len(statsINIs) > 0 {
		os.Exit(enableStatsAPI(statsINIs, *statsPort))
	}
	cfgPath, err := filepath.Abs(*cfgFlag)
	if err != nil {
		fatal("Konfiguration: %v", err)
	}
	instance := filepath.Join(filepath.Dir(cfgPath), "instance.json")

	single := desktop.SingleInstance(cfgPath)
	for i := 0; !single && *updated && i < 50; i++ { // the old version is still closing
		time.Sleep(200 * time.Millisecond)
		single = desktop.SingleInstance(cfgPath)
	}
	if !single {
		// started again by hand: bring up the interface of the running copy
		if !*atSignIn && showRunning(instance) != nil {
			desktop.ShowInfo(strings.TrimSpace("roMod läuft schon. " + desktop.TrayHint))
		}
		return
	}
	os.Remove(exe + ".old") // left by an update
	desktop.ForgetOldName()
	if err := renameOldFiles(filepath.Dir(cfgPath)); err != nil {
		fatal("Daten von auto-replay-upload übernehmen: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(cfgPath), "romod.log")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	restart := func() {
		if err := exec.Command(exe, "-updated", "-config", cfgPath).Start(); err != nil {
			log.Printf("Neustart nach Update: %v", err)
			return
		}
		stop()
	}

	d := &desk{pop: popup.New(), hotkeys: hk != nil, saveKey: game.SaveKeySupported()}
	a, err := app.New(app.Options{ConfigPath: cfgPath, LogPath: logPath, Desktop: d, Hotkeys: hk, ShowUI: !*atSignIn, Restart: restart})
	if err != nil {
		desktop.Edit(cfgPath)
		fatal("Start fehlgeschlagen: %v", err)
	}
	defer a.Close()
	d.url = a.URL()
	d.overlay = overlay.NewOverlay(d.url)
	d.replayDir = a.ReplayFolder
	if err := writeInstance(instance, d.url); err != nil {
		log.Printf("%s: %v", instance, err)
	}
	defer os.Remove(instance)

	d.autostart = []string{exe, "-autostart"}
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

// defaultConfig is data\config.json next to the program on Windows, so the
// whole installation is one folder. Elsewhere programs usually live in a
// folder the user cannot write to, so it is the user's configuration
// folder. A config.json right next to the program wins everywhere.
func defaultConfig(exe string) string {
	beside := filepath.Join(filepath.Dir(exe), "config.json")
	if _, err := os.Stat(beside); err == nil {
		return beside // ponytail: older Windows installations keep their files, moving them is up to the user
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(filepath.Dir(exe), "data", "config.json")
	}
	dir, err := os.UserConfigDir() // ~/Library/Application Support, ~/.config
	if err != nil {
		return beside
	}
	return filepath.Join(dir, "romod", "config.json")
}

// renameOldFiles gives the database and log of auto-replay-upload, as roMod
// was called until October 2026, the new name. The database goes last, so
// an interrupted rename is finished at the next start.
func renameOldFiles(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "romod.db")); err == nil {
		return nil
	}
	for _, ext := range []string{".log", ".log.old", ".db-wal", ".db-shm", ".db"} {
		err := os.Rename(filepath.Join(dir, "auto-replay-upload"+ext), filepath.Join(dir, "romod"+ext))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
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

func (d *desk) RunElevated(args ...string) error { return desktop.RunElevated(args...) }

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

// enableStatsAPI switches the Stats API on in the given ini files, for the
// interface, which starts the tool with administrator rights for this.
func enableStatsAPI(paths []string, port int) int {
	code := 0
	for _, p := range paths {
		if err := game.EnableStatsAPI(p, port); err != nil {
			desktop.ShowError("Stats API einschalten: " + err.Error())
			code = 1
		}
	}
	return code
}

// writeInstance notes where the running tool's interface is, so a second
// start can open it instead of starting another copy.
func writeInstance(path, uiURL string) error {
	data, _ := json.Marshal(map[string]string{"url": uiURL})
	return os.WriteFile(path, data, 0o600)
}

// showRunning asks the running copy to open its interface.
func showRunning(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var inst struct{ URL string }
	if err := json.Unmarshal(data, &inst); err != nil {
		return err
	}
	u, err := url.Parse(inst.URL)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, u.Scheme+"://"+u.Host+"/api/show", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Token", u.Query().Get("k"))
	res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return nil
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
