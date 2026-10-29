// Command uidemo serves the interface with made-up data against a fake
// ballchasing API, for working on the interface without an account. Nothing
// leaves the machine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/app"
	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing/ballchasingtest"
	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/mmr"
	"github.com/EYGjamie/roMod-Rocket-League/internal/store"
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
func (demoDesktop) HideQuickSort()              {}
func (demoDesktop) ShowLobby(l app.LobbyPopup)  { log.Printf("[Lobby] %s %v", l.Title, l.Teams) }
func (demoDesktop) HideLobby()                  {}
func (demoDesktop) Toast(t app.Toast)           { log.Printf("[Hinweis] %s: %s", t.Title, t.Body) }
func (demoDesktop) SetTray(p, f bool)           {}
func (demoDesktop) Recycle(path string) error   { return os.Remove(path) }
func (demoDesktop) Reveal(string) error         { return nil }
func (demoDesktop) OpenFolder(string) error     { return nil }
func (demoDesktop) Edit(string) error           { return nil }
func (demoDesktop) PickFiles() []string         { return nil }
func (demoDesktop) SetAutostart(bool) error     { return nil }
func (demoDesktop) Autostart() bool             { return true }
func (demoDesktop) RunElevated(...string) error { return errors.ErrUnsupported }
func (d demoDesktop) Platform() app.Platform    { return d.platform }

// platforms are what each system offers in a usual setup.
var platforms = map[string]app.Platform{
	"windows": {OS: "windows", Overlay: true, Tray: true, Autostart: true, Hotkeys: true, SaveKey: true, QuickSort: true},
	"linux":   {OS: "linux", Tray: true, Autostart: true, Hotkeys: true, SaveKey: true, QuickSort: true},
	"darwin":  {OS: "darwin", Tray: true, Autostart: true},
}

// slow delays every request to the fake, to see the interface wait.
type slow struct {
	next  http.RoundTripper
	delay time.Duration
}

func (s slow) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(s.delay)
	return s.next.RoundTrip(r)
}

func main() {
	setup := flag.Bool("setup", false, "mit dem Einrichtungsassistenten starten")
	delay := flag.Duration("slow", 0, "jede Anfrage an ballchasing so lange verzögern, z. B. 2s")
	dev := flag.Bool("dev", false, "mit Entwicklermodus, MMR-Abfrage und erfundenen Lobbys")
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

	// a game installation with the Stats API off, instead of the real one;
	// its port is the unused one below, so switching it on never reaches a
	// running game
	gameDir := filepath.Join(dir, "rocketleague")
	os.MkdirAll(filepath.Join(gameDir, "TAGame", "Config"), 0o755)
	os.WriteFile(filepath.Join(gameDir, "TAGame", "Config", "DefaultStatsAPI.ini"),
		[]byte("[TAGame.MatchStatsExporter_TA]\r\nPort=1\r\nWebPort=49124\r\nPacketSendRate=0"), 0o644)

	os.Unsetenv("BALLCHASING_TOKEN")
	cfg := config.Default()
	if !*setup {
		cfg.SetToken(ballchasingtest.Token)
		cfg.SetupDone = true
	}
	cfg.DemoDirs = []string{demos}
	cfg.StatsAPI = "127.0.0.1:1"
	cfg.AutoMark = false // never a key press in a real game
	cfg.DevMode, cfg.MMR = *dev, *dev
	if *dev {
		cfg.EpicName = "Zweitkonto"
	}
	cfgPath := filepath.Join(dir, "config.json")
	config.Save(cfgPath, cfg)

	client := fake.Client()
	if *delay > 0 {
		client.Transport = slow{client.Transport, *delay}
	}
	a, err := app.New(app.Options{ConfigPath: cfgPath, LogPath: filepath.Join(dir, "log.txt"), Desktop: demoDesktop{platform},
		BaseURL: fake.URL, HTTPClient: client, Unpaced: true, GameDirs: func() []string { return []string{gameDir} },
		MMR: func(o mmr.Options) app.MMR { return &demoMMR{save: o.Save, signedIn: *dev} }})
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	if !*setup {
		addRules(dir, threes)
	}
	if *dev {
		addLobbies(dir, "Steam|"+fake.Me.SteamID+"|0")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go a.Run(ctx)
	fmt.Println("URL " + a.URL())
	<-ctx.Done()
}

// addRules opens the demo database for a moment and adds rules for some
// playlists, so the rules page and the quick-sort flow have something, and
// two failed uploads.
func addRules(dir, threes string) {
	st, err := store.Open(filepath.Join(dir, "romod.db"))
	if err != nil {
		return
	}
	defer st.Close()
	st.PutFile(store.File{Name: "kaputt.replay", Dir: dir, State: store.Rejected, Error: "von ballchasing abgelehnt: failed to parse replay"})
	st.PutFile(store.File{Name: "weg.replay", Dir: dir, State: store.Failed, Attempts: 3, Error: "Datei nicht mehr vorhanden"})
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

// demoMMR signs in with any code and makes up ranks; nothing reaches Epic.
type demoMMR struct {
	save     func(token, name string)
	signedIn bool
}

func (m *demoMMR) Login(ctx context.Context, code string) (string, error) {
	m.signedIn = true
	m.save("demo", "Zweitkonto")
	return "Zweitkonto", nil
}
func (m *demoMMR) Logout()                       { m.signedIn = false; m.save("", "") }
func (m *demoMMR) SignedIn() bool                { return m.signedIn }
func (m *demoMMR) Connect(context.Context) error { return nil }
func (m *demoMMR) Close()                        {}
func (m *demoMMR) Version() mmr.Version          { return mmr.Builtin() }
func (m *demoMMR) Lookup(ctx context.Context, ids []string) (map[string]mmr.Player, error) {
	out := map[string]mmr.Player{}
	for _, id := range ids {
		v := 900 + rand.Float64()*500
		out[id] = mmr.Player{Ranks: [3]*mmr.Skill{{Tier: tierOf(v - 150), MMR: v - 150}, {Tier: tierOf(v), MMR: v}, {Tier: tierOf(v - 80), MMR: v - 80}}, Wins: rand.IntN(6000)}
	}
	return out, nil
}

// tierOf is roughly the rank of an MMR in 2s.
func tierOf(v float64) int { return min(22, max(1, int(v-100)/60)) }

// addLobbies stores a month of lobbies with ranks for the MMR page.
func addLobbies(dir, me string) {
	st, err := store.Open(filepath.Join(dir, "romod.db"))
	if err != nil {
		return
	}
	defer st.Close()
	r := rand.New(rand.NewPCG(3, 5))
	own := [3]float64{960, 1080, 1010}
	names := []string{"Zephyr", "Kiwi", "NoxRL", "Blitz", "Mango", "Rook", "Ghost", "Pixel", "Tornado", "Sly", "Max", "Lena"}
	at := time.Now().AddDate(0, 0, -30)
	for i := range 90 {
		at = at.Add(time.Duration(4+r.IntN(9)) * time.Hour / 2)
		if at.After(time.Now()) {
			break
		}
		size := []int{2, 2, 2, 3, 3, 1}[r.IntN(6)]
		smurf := r.IntN(7) == 0
		win := r.IntN(100) < 52
		own[size-1] += map[bool]float64{true: 9, false: -9}[win] + r.Float64()*2 - 1
		l := store.Lobby{GUID: fmt.Sprintf("DEMO%04d", i), Started: at, Size: size, Smurf: smurf, Winner: 1}
		if win {
			l.Winner = 0
		}
		player := func(id, name string, team int, base float64, isMe bool) store.LobbyPlayer {
			p := store.LobbyPlayer{ID: id, Name: name, Team: team, Me: isMe, Looked: true, Wins: 300 + r.IntN(5000)}
			for k := range 3 {
				v := base + float64(k-size+1)*40 + r.Float64()*30
				if isMe {
					v = own[k]
				}
				p.Ranks[k] = &store.Rank{Tier: tierOf(v), Div: r.IntN(4), MMR: v, Played: r.IntN(200)}
			}
			return p
		}
		l.Players = append(l.Players, player(me, "Jamie", 0, 0, true))
		for k := range 2*size - 1 {
			team := 1
			if k < size-1 {
				team = 0
			}
			base := own[size-1] + r.Float64()*120 - 50
			if smurf && k == 2*size-2 { // a fresh account, ranked far below how it plays
				l.Players = append(l.Players, store.LobbyPlayer{ID: fmt.Sprintf("Epic|s%d|0", i), Name: "xX_neu_Xx", Team: team, Looked: true, Wins: 40,
					Ranks: [3]*store.Rank{nil, {Tier: tierOf(own[size-1] - 200), MMR: own[size-1] - 200}, nil}})
				continue
			}
			l.Players = append(l.Players, player(fmt.Sprintf("Epic|%d-%d|0", i, k), names[r.IntN(len(names))], team, base, false))
		}
		st.PutLobby(l)
	}
}
