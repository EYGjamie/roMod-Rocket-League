// Command uidemo serves the interface with made-up data against a fake
// ballchasing API, for working on the interface without an account. Nothing
// leaves the machine.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/app"
	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing/ballchasingtest"
	"github.com/EYGjamie/auto-replay-upload/internal/config"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// cleanup collects what the fake wants to close; the fake expects a test.
type cleanup struct{ fns []func() }

func (c *cleanup) Errorf(format string, args ...any) { log.Printf("fake: "+format, args...) }
func (c *cleanup) Cleanup(f func())                  { c.fns = append(c.fns, f) }

// demoDesktop logs what would happen; platform says what the interface
// shows, e.g. the texts for macOS with -os darwin.
type demoDesktop struct{ platform app.Platform }

func (demoDesktop) ShowOverlay(route string) { log.Printf("[Overlay] %s", route) }
func (demoDesktop) ToggleOverlay()           {}
func (demoDesktop) OpenBrowser(t string)     { log.Printf("[Browser] %s", t) }
func (demoDesktop) ShowQuickSort(q app.QuickSort, pick func(int)) {
	log.Printf("[Einordnen] %s → %s %v", q.Title, q.Target, q.Options)
}
func (demoDesktop) HideQuickSort()            {}
func (demoDesktop) Toast(t app.Toast)         { log.Printf("[Hinweis] %s: %s", t.Title, t.Body) }
func (demoDesktop) SetTray(p, f bool)         {}
func (demoDesktop) Recycle(path string) error { return os.Remove(path) }
func (demoDesktop) Reveal(string) error       { return nil }
func (demoDesktop) OpenFolder(string) error   { return nil }
func (demoDesktop) Edit(string) error         { return nil }
func (demoDesktop) PickFiles() []string       { return nil }
func (demoDesktop) SetAutostart(bool) error   { return nil }
func (demoDesktop) Autostart() bool           { return true }
func (d demoDesktop) Platform() app.Platform  { return d.platform }

// platforms are what each system offers in a usual setup.
var platforms = map[string]app.Platform{
	"windows": {OS: "windows", Overlay: true, Tray: true, Autostart: true, Hotkeys: true, SaveKey: true, QuickSort: true},
	"linux":   {OS: "linux", Tray: true, Autostart: true, Hotkeys: true, SaveKey: true, QuickSort: true},
	"darwin":  {OS: "darwin", Tray: true, Autostart: true},
}

func main() {
	setup := flag.Bool("setup", false, "mit dem Einrichtungsassistenten starten")
	def := runtime.GOOS
	if _, ok := platforms[def]; !ok {
		def = "linux"
	}
	system := flag.String("os", def, "Oberfläche wie auf diesem System: windows, darwin oder linux")
	flag.Parse()
	platform, ok := platforms[*system]
	if !ok {
		log.Fatalf("-os %q unbekannt, erlaubt: windows, darwin, linux", *system)
	}

	var c cleanup
	defer func() {
		for _, f := range c.fns {
			f()
		}
	}()
	fake := ballchasingtest.New(&c)
	threes := seed(fake)

	dir, err := os.MkdirTemp("", "uidemo-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	demos := filepath.Join(dir, "Demos")
	os.MkdirAll(demos, 0o755)
	for i := range 5 {
		old := filepath.Join(demos, fmt.Sprintf("ALT%02d.replay", i))
		os.WriteFile(old, []byte("x"), 0o644)
		t := time.Now().AddDate(0, 0, -i*12)
		os.Chtimes(old, t, t)
	}

	os.Unsetenv("BALLCHASING_TOKEN")
	cfg := config.Default()
	if !*setup {
		cfg.SetToken(ballchasingtest.Token)
		cfg.SetupDone = true
	}
	cfg.DemoDirs = []string{demos}
	cfg.StatsAPI = "127.0.0.1:1"
	cfgPath := filepath.Join(dir, "config.json")
	config.Save(cfgPath, cfg)

	a, err := app.New(app.Options{ConfigPath: cfgPath, LogPath: filepath.Join(dir, "log.txt"), Desktop: demoDesktop{platform},
		BaseURL: fake.URL, HTTPClient: fake.Client(), Unpaced: true})
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	if !*setup {
		addRules(dir, threes)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go a.Run(ctx)
	fmt.Println("URL " + a.URL())
	<-ctx.Done()
}

// addRules opens the demo database for a moment and adds rules for some
// playlists, so the rules page and the quick-sort flow have something.
func addRules(dir, threes string) {
	st, err := store.Open(filepath.Join(dir, "auto-replay-upload.db"))
	if err != nil {
		return
	}
	defer st.Close()
	for _, r := range []store.Rule{
		{Enabled: true, Playlists: []string{"ranked-standard"}, Players: []string{"steam:3"}, TargetID: threes},
		{Enabled: true, Playlists: []string{"ranked-doubles"}, TargetName: "2s Ranked"},
		{Enabled: true, Playlists: []string{"private"}, TargetName: "Privat & Training"},
	} {
		st.PutRule(&r)
	}
}

var maps = []string{"DFH Stadium", "Mannfield", "Champions Field", "Urban Central", "Beckwith Park", "Utopia Coliseum", "Neo Tokyo", "Wasteland", "Salty Shores", "Forbidden Temple"}

// seed fills the fake with a month of games, groups and teammates' uploads.
func seed(f *ballchasingtest.Server) (threes string) {
	r := rand.New(rand.NewPCG(7, 11))
	me := "steam:" + f.Me.SteamID
	ranked := f.AddGroup(ballchasingtest.Group{Name: "Ranked", Created: time.Now().AddDate(0, -3, 0)})
	threes = f.AddGroup(ballchasingtest.Group{Name: "Threes", Parent: ranked})
	f.AddGroup(ballchasingtest.Group{Name: "Tourney", Parent: ranked})
	scrims := f.AddGroup(ballchasingtest.Group{Name: "Scrims"})
	scrimSubs := []string{
		f.AddGroup(ballchasingtest.Group{Name: "Scrim 2026-10-02 gegen Nova", Parent: scrims}),
		f.AddGroup(ballchasingtest.Group{Name: "Scrim 2026-10-09 gegen Flyback", Parent: scrims}),
	}
	liga := f.AddGroup(ballchasingtest.Group{Name: "Liga 2026"})
	spieltage := []string{
		f.AddGroup(ballchasingtest.Group{Name: "Spieltag 1", Parent: liga}),
		f.AddGroup(ballchasingtest.Group{Name: "Spieltag 2", Parent: liga}),
		f.AddGroup(ballchasingtest.Group{Name: "Spieltag 3", Parent: liga}),
	}
	opponents := []string{"Zephyr", "Kiwi", "NoxRL", "Blitz", "Mango", "Rook", "Ghost", "Pixel", "Tornado", "Sly"}
	player := func(name, pid string, strong bool) ballchasingtest.Player {
		p := ballchasingtest.Player{Name: name, PID: pid, Score: 150 + r.IntN(400), Shots: r.IntN(7), Saves: r.IntN(5), Assists: r.IntN(3)}
		if strong {
			p.Score += 120
		}
		return p
	}
	now := time.Now()
	for day := 29; day >= 0; day-- {
		if r.IntN(10) < 3 {
			continue // no games that day
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 18+r.IntN(3), r.IntN(60), 0, 0, time.Local).AddDate(0, 0, -day)
		games := 2 + r.IntN(6)
		for g := range games {
			at := start.Add(time.Duration(g*9+r.IntN(3)) * time.Minute)
			if at.After(now) {
				break
			}
			playlist := []string{"ranked-doubles", "ranked-doubles", "ranked-doubles", "ranked-standard", "ranked-standard", "ranked-duels", "private", "hoops", "unranked-doubles"}[r.IntN(9)]
			size := map[string]int{"ranked-duels": 1, "ranked-standard": 3}[playlist]
			if size == 0 {
				size = 2
			}
			win := r.IntN(100) < 56
			blue := []ballchasingtest.Player{player("Jamie", me, win)}
			if size >= 2 {
				blue = append(blue, player("Max", "steam:2", false))
			}
			if size == 3 {
				blue = append(blue, player("Lena", "steam:3", false))
			}
			var orange []ballchasingtest.Player
			for i := range size {
				name := opponents[r.IntN(len(opponents))]
				orange = append(orange, player(name, fmt.Sprintf("epic:%s%d", name, i), !win))
			}
			goals := func(ps []ballchasingtest.Player, n int) {
				for i := range n {
					ps[i%len(ps)].Goals++
				}
			}
			a, b := 1+r.IntN(4), r.IntN(3)
			if !win {
				a, b = b, a
			}
			if a == b {
				a++
			}
			goals(blue, a)
			goals(orange, b)
			group := ""
			switch {
			case day > 3 && playlist == "ranked-standard":
				group = threes
			case day > 3 && playlist == "private" && r.IntN(2) == 0:
				group = scrimSubs[r.IntN(2)]
			case day > 3 && playlist == "ranked-doubles" && r.IntN(3) > 0:
				group = spieltage[r.IntN(3)]
			case day > 3 && r.IntN(4) > 0:
				group = ranked
			}
			id := f.AddReplay(ballchasingtest.Replay{PlaylistID: playlist, Map: maps[r.IntN(len(maps))], Date: at,
				Created: at.Add(2 * time.Minute), Duration: 300 + r.IntN(90), Blue: blue, Orange: orange, Group: group,
				Visibility: []string{"public", "unlisted", "private"}[r.IntN(3)]})
			if r.IntN(8) == 0 { // Max uploaded the same game
				rp, _ := f.Replay(id)
				rp.ID, rp.UploaderID, rp.UploaderName, rp.Group, rp.Visibility = "", "222", "Max", "", "public"
				rp.Created = rp.Created.Add(time.Minute)
				f.AddReplay(rp)
			}
		}
	}
	for i := range 4 { // games only Max uploaded
		at := now.AddDate(0, 0, -i*3-1).Add(-2 * time.Hour)
		f.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: maps[i], Date: at, Created: at.Add(time.Minute),
			UploaderID: "222", UploaderName: "Max", Visibility: "public",
			Blue:   []ballchasingtest.Player{{Name: "Max", PID: "steam:2", Score: 410, Goals: 2, Shots: 4}, {Name: "Jamie", PID: me, Score: 380, Goals: 1, Shots: 3}},
			Orange: []ballchasingtest.Player{{Name: "Rook", PID: "epic:rook", Score: 220, Goals: i % 3, Shots: 3}, {Name: "Sly", PID: "epic:sly", Score: 150, Shots: 1}}})
	}
	return threes
}
