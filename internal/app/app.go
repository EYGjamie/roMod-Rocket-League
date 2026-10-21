// Package app ties the parts together: upload queue, ballchasing sync,
// rules, sessions, the match tracker and the local web interface.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EYGjamie/romod/internal/ballchasing"
	"github.com/EYGjamie/romod/internal/config"
	"github.com/EYGjamie/romod/internal/game"
	"github.com/EYGjamie/romod/internal/hotkey"
	"github.com/EYGjamie/romod/internal/store"
	"github.com/EYGjamie/romod/internal/upload"
)

// Desktop is what the app needs from the system side. Tests use a fake.
type Desktop interface {
	ShowOverlay(route string)
	ToggleOverlay()
	OpenBrowser(route string)
	ShowQuickSort(q QuickSort, pick func(choice int))
	HideQuickSort()
	Toast(t Toast)
	SetTray(paused, failed bool)
	Recycle(path string) error
	Reveal(path string) error
	OpenFolder(dir string) error
	Edit(path string) error
	PickFiles() []string
	SetAutostart(on bool) error
	Autostart() bool
	// RunElevated runs the tool again with args and administrator rights;
	// errors.ErrUnsupported where the system has no such prompt.
	RunElevated(args ...string) error
	Platform() Platform
}

// Platform tells the interface what the system side offers.
type Platform struct {
	OS        string `json:"os"`         // windows, darwin or linux
	Overlay   bool   `json:"overlay"`    // an own window over the game, else the browser
	Tray      bool   `json:"tray"`       // an icon in the notification area or menu bar
	Autostart bool   `json:"autostart"`  // starting at sign-in can be switched
	Hotkeys   bool   `json:"hotkeys"`    // system-wide hotkeys
	SaveKey   bool   `json:"save_key"`   // the game's save key can be pressed
	QuickSort bool   `json:"quick_sort"` // the quick-sort prompt takes a choice
}

// QuickSort is the prompt shown over the game after an upload.
type QuickSort struct {
	Title   string    // e.g. "2s Ranked · DFH Stadium · Sieg 3:1"
	Target  string    // folder the rule chose
	Options [4]string // labels for Ctrl+Alt+1–4, "" if unused
	Seconds int
}

// Toast is a notification with up to two actions.
type Toast struct {
	Kind    string // uploaded, failed, session, statsapi
	Title   string
	Body    string
	Actions []ToastAction
	Disable func() // "don't show again"
}

type ToastAction struct {
	Label string
	Run   func()
}

// Options configure New.
type Options struct {
	ConfigPath  string
	LogPath     string
	Desktop     Desktop
	Hotkeys     *hotkey.Manager // nil disables hotkeys (tests)
	BaseURL     string          // ballchasing, default https://ballchasing.com
	HTTPClient  *http.Client
	Unpaced     bool // no client-side rate limiting (tests)
	Now         func() time.Time
	GameDirs    func() []string // the game's install folders, default game.GameDirs
	GameRunning func() bool     // whether Rocket League runs, default game.RocketLeagueRunning
	ShowUI      bool            // open the interface at start, not only after sign-in without start_minimized
	Restart     func()          // starts the updated program and ends this one; nil opens the release page instead
}

type App struct {
	opts    Options
	dataDir string
	desk    Desktop
	store   *store.Store
	queue   *upload.Queue
	tracker *game.Tracker
	events  *hub
	quick   *quickSorter
	now     func() time.Time

	mu       sync.RWMutex
	cfg      config.Config
	client   *ballchasing.Client
	me       ballchasing.Me
	account  string // none, ok, invalid, offline
	paused   struct{ watch, mark bool }
	toasts   []Toast // held back during a match
	lastGame time.Time
	notified time.Time // end of the last session announced
	hint     struct {
		since time.Time // Rocket League runs without the Stats API since
		done  bool      // looked for the reason during this start of the game
	}

	statsAPI atomic.Bool
	syncing  atomic.Bool
	release  atomic.Pointer[release] // newer release on GitHub, nil if none
	syncWake chan bool               // true = full sync

	ln    net.Listener
	token string
	ctx   context.Context
}

// New loads the settings and the database and opens the local port. Run
// starts everything else.
func New(opts Options) (*App, error) {
	if opts.BaseURL == "" {
		opts.BaseURL = "https://ballchasing.com"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 2 * time.Minute}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	a := &App{opts: opts, dataDir: filepath.Dir(opts.ConfigPath), desk: opts.Desktop, events: newHub(), notified: opts.Now(),
		now: opts.Now, account: "none", syncWake: make(chan bool, 1), ctx: context.Background()}
	a.quick = &quickSorter{a: a}

	cfg, err := config.Load(opts.ConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := config.Save(opts.ConfigPath, cfg); err != nil {
			return nil, err
		}
		log.Printf("%s angelegt", opts.ConfigPath)
	} else if err != nil {
		return nil, err
	}
	a.cfg = cfg
	if cfg.Token != "" {
		a.client = a.newClient(cfg.Token)
		a.account = "offline" // until the first ping
	}

	if a.store, err = store.Open(filepath.Join(a.dataDir, "romod.db")); err != nil {
		return nil, err
	}
	if err := a.importLegacy(); err != nil {
		log.Printf("uploaded.json übernehmen: %v", err)
	}

	a.queue = upload.New(upload.Config{
		Store: a.store,
		API: func() upload.API {
			if c := a.currentClient(); c != nil {
				return c
			}
			return nil
		},
		Dirs:       a.demoDirs,
		Visibility: func() string { return a.config().Visibility },
		Processed:  a.processed,
		Failed:     a.uploadFailed,
		Changed:    func() { a.events.publish("queue"); a.updateTray() },
	})
	a.tracker = game.NewTracker(trackerOptions(cfg), game.Hooks{
		MatchStarted: func() { a.events.publish("state") },
		MatchEnded:   a.matchEnded,
		ReplayLeft:   a.cleanBorrows,
	})

	b := make([]byte, 16)
	rand.Read(b)
	a.token = hex.EncodeToString(b)
	if a.ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *App) newClient(token string) *ballchasing.Client {
	return &ballchasing.Client{BaseURL: a.opts.BaseURL, Token: token, HTTPClient: a.opts.HTTPClient, Unpaced: a.opts.Unpaced}
}

func trackerOptions(c config.Config) game.Options {
	return game.Options{AutoMark: c.AutoMark, OnlineOnly: c.OnlineOnly, Key: c.MarkKey, Hold: time.Duration(c.MarkHoldMs) * time.Millisecond}
}

// URL is the address of the interface, including the per-run key.
func (a *App) URL() string {
	return "http://" + a.ln.Addr().String() + "/?k=" + a.token
}

// Close releases the database.
func (a *App) Close() error {
	a.ln.Close()
	return a.store.Close()
}

func (a *App) config() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *App) currentClient() *ballchasing.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

func (a *App) myAccount() ballchasing.Me {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.me
}

// demoDirs returns the configured replay folders or the default ones.
func (a *App) demoDirs() []string {
	if dirs := a.config().DemoDirs; len(dirs) > 0 {
		return dirs
	}
	dirs, _ := game.DefaultDemoDirs()
	return dirs
}

// importLegacy takes over uploaded.json of earlier versions once.
func (a *App) importLegacy() error {
	path := filepath.Join(a.dataDir, "uploaded.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var old struct {
		Uploaded map[string]string `json:"uploaded"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return err
	}
	if err := a.store.ImportLegacy(old.Uploaded); err != nil {
		return err
	}
	a.store.SetMeta("seeded", "1")
	log.Printf("%d Einträge aus uploaded.json übernommen", len(old.Uploaded))
	return os.Rename(path, path+".bak")
}

// Run starts the background work and serves the interface until ctx is
// done.
func (a *App) Run(ctx context.Context) {
	a.ctx = ctx
	srv := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 10 * time.Second}
	context.AfterFunc(ctx, func() { srv.Close() })
	go func() {
		if err := srv.Serve(a.ln); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Oberfläche beendet: %v", err)
		}
	}()

	if a.store.Meta("seeded") == "" {
		n, err := a.queue.Seed()
		if err != nil {
			log.Printf("Vorhandene Replays registrieren: %v", err)
		} else {
			log.Printf("Erster Start: %d vorhandene Replays registriert, nicht hochgeladen", n)
			a.store.SetMeta("seeded", "1")
		}
	}
	if a.config().UploadExisting {
		if n, _ := a.queue.QueueSeeded(time.Time{}); n > 0 {
			log.Printf("%d ältere Replays werden nachträglich hochgeladen", n)
		}
	}
	a.cleanOldBorrows()
	a.bindHotkeys(config.Config{}, a.config())

	go a.queue.Run(ctx)
	go game.ListenStatsAPI(ctx, func() string { return a.config().StatsAPI }, a.tracker.Handle, a.statsAPIConnected)
	go a.syncLoop(ctx)
	go a.detailLoop(ctx)
	go a.housekeeping(ctx)
	go a.autoEnableStatsAPI()
	go a.watchUpdates(ctx)

	if !a.config().SetupDone {
		a.desk.ShowOverlay("einrichtung")
	} else if a.opts.ShowUI || !a.config().StartMinimized {
		a.desk.ShowOverlay("uebersicht")
	}
	a.updateTray()
	<-ctx.Done()
}

// housekeeping releases held notifications, announces ended sessions and
// notices a Stats API that is off.
func (a *App) housekeeping(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.flushToasts()
			a.checkSessionEnd(false)
			a.checkStatsAPI()
		}
	}
}

func (a *App) statsAPIConnected(up bool) {
	a.statsAPI.Store(up)
	a.events.publish("state")
	if !up {
		// Rocket League closed: the session is over
		time.AfterFunc(5*time.Second, func() {
			if !a.gameRunning() {
				a.checkSessionEnd(true)
			}
		})
	}
}

func (a *App) matchEnded(marked bool) {
	a.mu.Lock()
	a.lastGame = a.now()
	a.mu.Unlock()
	a.events.publish("state")
	time.AfterFunc(3*time.Second, a.flushToasts)
}

// SetPaused pauses or resumes watching (and uploading) and marking.
func (a *App) SetPaused(watch, mark bool) {
	a.mu.Lock()
	a.paused.watch, a.paused.mark = watch, mark
	a.mu.Unlock()
	a.queue.SetPaused(watch)
	a.tracker.SetPaused(mark)
	switch {
	case watch && mark:
		log.Print("Pausiert: keine Uploads, kein Auto-Speichern")
	case watch:
		log.Print("Überwachung pausiert")
	case mark:
		log.Print("Auto-Speichern pausiert")
	default:
		log.Print("Läuft wieder")
	}
	a.events.publish("state")
	a.updateTray()
}

// TogglePause pauses both if anything runs, otherwise resumes both.
func (a *App) TogglePause() {
	a.mu.RLock()
	running := !a.paused.watch || !a.paused.mark
	a.mu.RUnlock()
	a.SetPaused(running, running)
}

func (a *App) pausedState() (watch, mark bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.paused.watch, a.paused.mark
}

func (a *App) updateTray() {
	watch, mark := a.pausedState()
	counts, _ := a.store.CountFiles()
	a.desk.SetTray(watch && mark, counts[store.Failed]+counts[store.Rejected] > 0)
}

// bindHotkeys registers the overlay and pause hotkeys that changed.
func (a *App) bindHotkeys(old, cur config.Config) {
	if a.opts.Hotkeys == nil {
		return
	}
	bind := func(name, oldKey, key string, fn func()) {
		if oldKey == key {
			return
		}
		hk, _ := hotkey.Parse(key)
		if err := a.opts.Hotkeys.Set(name, hk, fn); err != nil {
			log.Printf("Hotkey %s nicht verfügbar: %v", key, err)
		} else if !hk.IsZero() {
			open := "Overlay öffnen und schließen"
			if !a.desk.Platform().Overlay {
				open = "Oberfläche öffnen"
			}
			log.Printf("%s: %s", key, map[string]string{"overlay": open, "pause": "Pausieren und fortsetzen"}[name])
		}
	}
	bind("overlay", old.GUIHotkey, cur.GUIHotkey, a.desk.ToggleOverlay)
	bind("pause", old.PauseHotkey, cur.PauseHotkey, a.TogglePause)
}

// UpdateSettings applies a partial settings object as sent by the
// interface and saves it.
func (a *App) UpdateSettings(patch []byte) (config.Config, error) {
	a.mu.Lock()
	old := a.cfg
	next := a.cfg
	next.DemoDirs = slices.Clone(old.DemoDirs) // Unmarshal would write into old's array
	if err := json.Unmarshal(patch, &next); err != nil {
		a.mu.Unlock()
		return old, err
	}
	next.Normalize()
	next.Token, next.TokenProtected = old.Token, old.TokenProtected // only via SetToken
	if err := next.Validate(); err != nil {
		a.mu.Unlock()
		return old, err
	}
	if err := config.Save(a.opts.ConfigPath, next); err != nil {
		a.mu.Unlock()
		return old, err
	}
	a.cfg = next
	a.mu.Unlock()

	a.tracker.SetOptions(trackerOptions(next))
	a.bindHotkeys(old, next)
	a.events.publish("settings")
	return next, nil
}

// SetToken checks a new ballchasing token and saves it if it works.
func (a *App) SetToken(ctx context.Context, token string) (ballchasing.Me, error) {
	c := a.newClient(token)
	me, err := c.Ping(ctx)
	if err != nil {
		return me, err
	}
	a.mu.Lock()
	a.cfg.SetToken(token)
	err = config.Save(a.opts.ConfigPath, a.cfg)
	a.client, a.me, a.account = c, me, "ok"
	a.mu.Unlock()
	if err != nil {
		return me, err
	}
	log.Printf("Bei ballchasing angemeldet als %s", me.Name)
	a.queue.TokenChanged()
	a.requestSync(true)
	a.events.publish("state")
	return me, nil
}

// RemoveToken disconnects from ballchasing.
func (a *App) RemoveToken() error {
	a.mu.Lock()
	a.cfg.SetToken("")
	err := config.Save(a.opts.ConfigPath, a.cfg)
	a.client, a.me, a.account = nil, ballchasing.Me{}, "none"
	a.mu.Unlock()
	a.events.publish("state")
	return err
}

// ping checks the token in the background and records the account.
func (a *App) ping(ctx context.Context) {
	c := a.currentClient()
	if c == nil {
		return
	}
	me, err := c.Ping(ctx)
	a.mu.Lock()
	before := a.account + a.me.Name
	switch {
	case err == nil:
		a.me, a.account = me, "ok"
	case errors.Is(err, ballchasing.ErrBadToken):
		a.account = "invalid"
	default:
		a.account = "offline"
	}
	changed := a.account+a.me.Name != before
	a.mu.Unlock()
	switch {
	case !changed:
	case err == nil:
		log.Printf("Bei ballchasing angemeldet als %s", me.Name)
	case errors.Is(err, ballchasing.ErrBadToken):
		log.Print("Der ballchasing-Token ist ungültig, bitte in den Einstellungen einen neuen eintragen")
	default:
		log.Printf("ballchasing gerade nicht erreichbar (%v)", err)
	}
	a.events.publish("state")
}

// countUpload adds a new upload to the 24-hour quota until the next ping
// brings the real count.
func (a *App) countUpload() {
	a.mu.Lock()
	a.me.Quota.Uploads.Used++
	a.mu.Unlock()
	a.events.publish("state")
}

// myIDs returns the user's player IDs.
func (a *App) myIDs() map[string]bool {
	ids, _ := a.store.Identities()
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id.PID] = true
	}
	return out
}

// Config returns the current settings.
func (a *App) Config() config.Config { return a.config() }
