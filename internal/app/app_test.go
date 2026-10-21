package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing"
	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing/ballchasingtest"
	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
	"github.com/EYGjamie/roMod-Rocket-League/internal/rules"
	"github.com/EYGjamie/roMod-Rocket-League/internal/store"
)

type fakeDesk struct {
	mu        sync.Mutex
	quick     []QuickSort
	pick      func(int)
	toasts    []Toast
	recycled  []string
	overlay   []string
	autostart bool
	quickSort bool // the prompt takes a choice
}

func (d *fakeDesk) ShowOverlay(route string) {
	d.mu.Lock()
	d.overlay = append(d.overlay, route)
	d.mu.Unlock()
}
func (d *fakeDesk) ToggleOverlay()     {}
func (d *fakeDesk) OpenBrowser(string) {}
func (d *fakeDesk) ShowQuickSort(q QuickSort, pick func(int)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.quick = append(d.quick, q)
	d.pick = pick
}
func (d *fakeDesk) HideQuickSort()              {}
func (d *fakeDesk) Toast(t Toast)               { d.mu.Lock(); d.toasts = append(d.toasts, t); d.mu.Unlock() }
func (d *fakeDesk) SetTray(bool, bool)          {}
func (d *fakeDesk) Reveal(string) error         { return nil }
func (d *fakeDesk) OpenFolder(string) error     { return nil }
func (d *fakeDesk) Edit(string) error           { return nil }
func (d *fakeDesk) PickFiles() []string         { return nil }
func (d *fakeDesk) Autostart() bool             { return d.autostart }
func (d *fakeDesk) SetAutostart(on bool) error  { d.autostart = on; return nil }
func (d *fakeDesk) RunElevated(...string) error { return errors.ErrUnsupported }
func (d *fakeDesk) Platform() Platform {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Platform{OS: "test", Autostart: true, Hotkeys: true, SaveKey: true, QuickSort: d.quickSort}
}
func (d *fakeDesk) Recycle(path string) error {
	d.mu.Lock()
	d.recycled = append(d.recycled, path)
	d.mu.Unlock()
	return os.Remove(path)
}

func (d *fakeDesk) snapshot() (quick []QuickSort, toasts []Toast, recycled []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.quick), slices.Clone(d.toasts), slices.Clone(d.recycled)
}

type testApp struct {
	*App
	t    *testing.T
	fake *ballchasingtest.Server
	desk *fakeDesk
	dir  string // replay folder
	game string // the game's install folder, empty
	srv  string // base URL of the interface

	running atomic.Bool  // Rocket League runs, never the real process
	skew    atomic.Int64 // added to the app's clock
}

// advance moves the app's clock forward.
func (ta *testApp) advance(d time.Duration) { ta.skew.Add(int64(d)) }

func newTestApp(t *testing.T, mutate func(*config.Config)) *testApp {
	t.Helper()
	t.Setenv("BALLCHASING_TOKEN", "")
	fake := ballchasingtest.New(t)
	data, demos := t.TempDir(), t.TempDir()
	cfg := config.Default()
	cfg.SetToken(ballchasingtest.Token)
	cfg.DemoDirs = []string{demos}
	cfg.SetupDone = true
	cfg.StatsAPI = "127.0.0.1:1"
	if mutate != nil {
		mutate(&cfg)
	}
	cfgPath := filepath.Join(data, "config.json")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	desk := &fakeDesk{quickSort: true}
	game := t.TempDir() // never the real installation
	ta := &testApp{t: t, fake: fake, desk: desk, dir: demos, game: game}
	a, err := New(Options{ConfigPath: cfgPath, LogPath: filepath.Join(data, "log.txt"), Desktop: desk,
		BaseURL: fake.URL, HTTPClient: fake.Client(), Unpaced: true, GameDirs: func() []string { return []string{game} },
		GameRunning: ta.running.Load, Now: func() time.Time { return time.Now().Add(time.Duration(ta.skew.Load())) }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); time.Sleep(50 * time.Millisecond); a.Close() })
	go a.Run(ctx)
	ta.App, ta.srv = a, "http://"+a.ln.Addr().String()
	ta.waitFor("account ok", func() bool { return a.accountInfo().Status == "ok" })
	ta.waitFor("first sync", func() bool { return !a.Syncing() }) // else a test's own sync is skipped
	a.store.SetIdentities([]store.Identity{{PID: "steam:" + fake.Me.SteamID, Name: "Jamie", Confirmed: true}})
	return ta
}

func (ta *testApp) waitFor(what string, cond func() bool) {
	ta.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			ta.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// drop puts a replay file into the replay folder and queues it.
func (ta *testApp) drop(name string) {
	ta.t.Helper()
	if err := os.WriteFile(filepath.Join(ta.dir, name), []byte("replay"), 0o644); err != nil {
		ta.t.Fatal(err)
	}
	ta.store.PutFile(store.File{Name: name, Dir: ta.dir, State: store.Queued})
	ta.queue.Wake()
}

func (ta *testApp) fileState(name string) store.File {
	f, _, _ := ta.store.File(name)
	return f
}

// call sends a request to the interface and decodes the reply into out.
func (ta *testApp) call(method, path, body string, out any) int {
	ta.t.Helper()
	req, _ := http.NewRequest(method, ta.srv+path, strings.NewReader(body))
	req.Header.Set("X-Token", ta.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ta.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			ta.t.Fatalf("%s %s: %v in %s", method, path, err, data)
		}
	}
	return resp.StatusCode
}

func (ta *testApp) setDefaultRules() {
	for _, r := range rules.Defaults() {
		ta.store.PutRule(&r)
	}
}

func TestUploadIsSortedByRuleAfterQuickSort(t *testing.T) {
	ta := newTestApp(t, nil)
	ta.setDefaultRules()

	ta.drop("ranked-doubles_a.replay")
	ta.waitFor("quick sort prompt", func() bool { q, _, _ := ta.desk.snapshot(); return len(q) == 1 })
	q, _, _ := ta.desk.snapshot()
	if q[0].Target != "2s Ranked" || q[0].Options[3] != "Eingang" || !strings.Contains(q[0].Title, "Sieg 3:1") {
		t.Fatalf("prompt = %+v", q[0])
	}
	if g, ok := ta.fake.GroupByName("2s Ranked"); !ok || g.Parent != "" {
		t.Fatal("rule target was not created on ballchasing")
	}
	ta.desk.pick(0) // countdown over
	f := ta.fileState("ranked-doubles_a.replay")
	ta.waitFor("group set", func() bool { r, _ := ta.fake.Replay(f.ReplayID); return r.Group != "" })

	rep, _, _ := ta.store.Replay(f.ReplayID)
	if !rep.Own || rep.GroupName != "2s Ranked" || !rep.HasDetail || rep.File != "ranked-doubles_a.replay" {
		t.Errorf("stored replay = %+v", rep)
	}
	_, toasts, recycled := ta.desk.snapshot()
	if len(toasts) != 1 || !strings.HasSuffix(toasts[0].Body, "→ 2s Ranked") {
		t.Errorf("toasts = %+v", toasts)
	}
	if len(recycled) != 0 {
		t.Error("deleted although delete_after_upload is off")
	}
	decisions, _ := ta.store.Decisions()
	if len(decisions) != 1 || decisions[0].Manual || decisions[0].PlaylistID != "ranked-doubles" {
		t.Errorf("decisions = %+v", decisions)
	}

	// second upload: pick the inbox
	ta.drop("ranked-doubles_b.replay")
	ta.waitFor("second prompt", func() bool { q, _, _ := ta.desk.snapshot(); return len(q) == 2 })
	ta.desk.pick(4)
	b := ta.fileState("ranked-doubles_b.replay")
	ta.waitFor("inbox decision", func() bool { _, toasts, _ := ta.desk.snapshot(); return len(toasts) == 2 })
	if r, _ := ta.fake.Replay(b.ReplayID); r.Group != "" {
		t.Errorf("inbox choice moved the replay to %q", r.Group)
	}
	var inbox struct {
		Sessions []inboxSession `json:"sessions"`
	}
	ta.call("GET", "/api/inbox", "", &inbox)
	if len(inbox.Sessions) != 1 || len(inbox.Sessions[0].Items) != 1 || inbox.Sessions[0].Items[0].Suggestions[0].Name != "2s Ranked" {
		t.Errorf("inbox = %+v", inbox)
	}
}

func TestDeleteAfterUploadAndQuickSortOff(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.DeleteUploaded, c.QuickSort = true, false })
	ta.setDefaultRules()
	ta.drop("private_a.replay")
	ta.waitFor("upload done", func() bool { return ta.fileState("private_a.replay").Deleted })
	f := ta.fileState("private_a.replay")
	if _, err := os.Stat(filepath.Join(ta.dir, "private_a.replay")); !os.IsNotExist(err) {
		t.Error("file still on disk")
	}
	ta.waitFor("sorted without prompt", func() bool { r, _ := ta.fake.Replay(f.ReplayID); return r.Group != "" })
	if q, _, _ := ta.desk.snapshot(); len(q) != 0 {
		t.Error("prompt shown although quick sort is off")
	}

	manual := filepath.Join(t.TempDir(), "private_m.replay")
	os.WriteFile(manual, []byte("x"), 0o644)
	ta.UploadFiles([]string{manual})
	ta.waitFor("manual upload", func() bool { return ta.fileState("private_m.replay").State == store.Done })
	if _, err := os.Stat(manual); err != nil {
		t.Error("a file picked by hand was deleted")
	}
	// the file is Done before the processed hook counts the upload
	ta.waitFor("upload counted", func() bool { return ta.accountInfo().Uploads.Used == 2 })
	if acc := ta.accountInfo(); acc.Tier != "regular" || acc.Uploads != (ballchasing.Usage{Used: 2, Max: 10}) {
		t.Errorf("account after two uploads = %+v", acc)
	}
}

// Where the prompt cannot take a choice (macOS), rules sort right away.
func TestRulesSortRightAwayWithoutPrompt(t *testing.T) {
	ta := newTestApp(t, nil)
	ta.desk.mu.Lock()
	ta.desk.quickSort = false
	ta.desk.mu.Unlock()
	ta.setDefaultRules()
	ta.drop("ranked-doubles_a.replay")
	f := ta.fileState("ranked-doubles_a.replay")
	ta.waitFor("sorted without prompt", func() bool {
		f = ta.fileState("ranked-doubles_a.replay")
		r, _ := ta.fake.Replay(f.ReplayID)
		return f.ReplayID != "" && r.Group != ""
	})
	if q, _, _ := ta.desk.snapshot(); len(q) != 0 {
		t.Error("prompt shown although it cannot take a choice")
	}
	var state struct {
		Platform Platform `json:"platform"`
	}
	ta.call("GET", "/api/state", "", &state)
	if state.Platform.OS != "test" || state.Platform.QuickSort {
		t.Errorf("platform in /api/state = %+v", state.Platform)
	}
}

func TestSyncMergesOwnAndMatesReplays(t *testing.T) {
	ta := newTestApp(t, nil)
	me := "steam:" + ta.fake.Me.SteamID
	base := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	players := func() ([]ballchasingtest.Player, []ballchasingtest.Player) {
		return []ballchasingtest.Player{{Name: "Jamie", PID: me, Goals: 2}, {Name: "Max", PID: "steam:2"}},
			[]ballchasingtest.Player{{Name: "Gegner", PID: "epic:x", Goals: 1}}
	}
	b, o := players()
	own := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: "Mannfield", Date: base, Created: base.Add(time.Minute), Blue: b, Orange: o})
	b, o = players()
	dup := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: "Mannfield", Date: base.Add(20 * time.Second),
		Created: base.Add(2 * time.Minute), UploaderID: "222", UploaderName: "Max", Visibility: "public", Blue: b, Orange: o})
	b, o = players()
	other := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private", Map: "Utopia", Date: base.Add(time.Hour),
		Created: base.Add(time.Hour), UploaderID: "222", UploaderName: "Max", Visibility: "unlisted", Blue: b, Orange: o})
	ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private", Map: "Utopia", Date: base.Add(2 * time.Hour),
		UploaderID: "333", UploaderName: "Fremd", Visibility: "private", Blue: b, Orange: o})
	liga := ta.fake.AddGroup(ballchasingtest.Group{Name: "Liga"})
	ta.fake.AddGroup(ballchasingtest.Group{Name: "Spieltag 1", Parent: liga})

	ta.syncAll(context.Background(), true)

	var list listResult
	ta.call("GET", "/api/replays?status=uploaded", "", &list)
	var ids []string
	for _, it := range list.Items {
		ids = append(ids, it.(map[string]any)["id"].(string))
	}
	if !slices.Equal(ids, []string{other, own}) {
		t.Errorf("history = %v, want %v (own wins over %s, private ones of others invisible)", ids, []string{other, own}, dup)
	}
	var detail struct {
		Replay replayView   `json:"replay"`
		Also   []replayView `json:"also"`
	}
	ta.call("GET", "/api/replays/"+own, "", &detail)
	if !detail.Replay.HasDetail || len(detail.Also) != 1 || detail.Also[0].Uploader != "Max" || detail.Replay.Result != "win" {
		t.Errorf("detail = %+v", detail)
	}
	groups, _ := ta.store.Groups()
	if len(groups) != 2 || ta.groupPath(groups, groups[1].ID) != "Liga › Spieltag 1" && ta.groupPath(groups, groups[0].ID) != "Liga › Spieltag 1" {
		t.Errorf("groups = %+v", groups)
	}
	if code := ta.call("POST", "/api/replays/move", `{"ids":["`+other+`"],"group":"`+liga+`"}`, &struct{}{}); code != http.StatusOK {
		t.Errorf("move status %d", code)
	}
	if r, _ := ta.fake.Replay(other); r.Group != "" {
		t.Error("a replay of someone else was moved")
	}
}

func TestDedupeAfterOneReplay(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{store: st}
	base := time.Date(2026, 10, 18, 20, 0, 0, 0, time.Local)
	game := func(id string, own bool, created time.Duration) store.Replay {
		return store.Replay{ID: id, Own: own, MapCode: "stadium_p", BlueGoals: 2, OrangeGoals: 1, Date: base, Created: base.Add(created),
			Players: []store.Player{{Team: "blue", PID: "steam:1"}, {Team: "orange", PID: "epic:2"}}}
	}
	dupOf := func(id string) string {
		r, _, _ := st.Replay(id)
		return r.DupOf
	}
	st.PutReplay(game("mate", false, time.Minute))
	a.dedupeAfter(store.Replay{}, "mate")
	st.PutReplay(game("own", true, 2*time.Minute))
	a.dedupeAfter(store.Replay{}, "own")
	if dupOf("mate") != "own" || dupOf("own") != "" {
		t.Fatalf("mate → %q, own → %q; want the own upload shown", dupOf("mate"), dupOf("own"))
	}

	// the own upload turns out to be another game: the mate's is shown again
	old, _, _ := st.Replay("own")
	other := game("own", true, 2*time.Minute)
	other.OrangeGoals = 3
	st.PutReplay(other)
	a.dedupeAfter(old, "own")
	if dupOf("mate") != "" || dupOf("own") != "" {
		t.Errorf("after the score changed: mate → %q, own → %q", dupOf("mate"), dupOf("own"))
	}
}

func TestIdentityGuess(t *testing.T) {
	ta := newTestApp(t, nil)
	ta.store.SetIdentities(nil)
	day := time.Now().Add(-24 * time.Hour)
	for i := range 4 {
		ta.store.PutReplay(store.Replay{ID: "s" + string(rune('a'+i)), Own: true, Date: day.Add(time.Duration(i) * time.Hour),
			Players: []store.Player{{Team: "blue", PID: "steam:1", Name: "Jamie"}, {Team: "orange", PID: "epic:o" + string(rune('a'+i))}}})
	}
	for i := range 2 {
		ta.store.PutReplay(store.Replay{ID: "e" + string(rune('a'+i)), Own: true, Date: day.Add(time.Duration(10+i) * time.Hour),
			Players: []store.Player{{Team: "blue", PID: "epic:jamie", Name: "JamieEpic"}, {Team: "orange", PID: "epic:p" + string(rune('a'+i))}}})
	}
	got := ta.identityCandidates()
	if len(got) != 2 || got[0].PID != "steam:1" || got[1].PID != "epic:jamie" {
		t.Errorf("identityCandidates() = %+v", got)
	}
}

func TestStatsAndSessions(t *testing.T) {
	ta := newTestApp(t, nil)
	me := "steam:" + ta.fake.Me.SteamID
	now := time.Now()
	add := func(id string, at time.Time, win bool, playlist, mapName string) {
		blue, orange := 3, 1
		if !win {
			blue, orange = 0, 2
		}
		ta.store.PutReplay(store.Replay{ID: id, Own: true, Date: at, Created: at, PlaylistID: playlist, MapName: mapName,
			BlueGoals: blue, OrangeGoals: orange, Players: []store.Player{{Team: "blue", PID: me}, {Team: "orange", PID: "epic:x"}}})
	}
	yesterday := now.AddDate(0, 0, -1)
	add("a", yesterday, true, "ranked-doubles", "Mannfield")
	add("b", yesterday.Add(10*time.Minute), true, "ranked-doubles", "Mannfield")
	add("c", yesterday.Add(20*time.Minute), false, "ranked-doubles", "Mannfield")
	add("d", yesterday.Add(3*time.Hour), true, "private", "Utopia") // new session
	add("old", now.AddDate(0, 0, -20), false, "private", "Utopia")

	st := ta.Stats(7)
	if st.Games != 4 || st.Wins != 3 || st.Losses != 1 || st.Uploads != 4 || len(st.PerPeriod) != 7 {
		t.Fatalf("Stats(7) = %+v", st)
	}
	if st.Playlists[0].ID != "ranked-doubles" || st.Playlists[0].Wins != 2 || len(st.Maps) != 1 || st.Maps[0].Name != "Mannfield" {
		t.Errorf("playlists %+v, maps %+v", st.Playlists, st.Maps)
	}
	if all := ta.Stats(0); all.Games != 5 || all.Unit != "month" {
		t.Errorf("Stats(all) = %d games, unit %s", all.Games, all.Unit)
	}
	sessions := sessionsOf(ta.visible(), ta.myIDs())
	if len(sessions) != 3 || sessions[1].Games != 3 || sessions[1].Streak != 2 || !slices.Equal(sessions[1].Results, []string{"win", "win", "loss"}) {
		t.Errorf("sessions = %+v", sessions)
	}
	if s, ok := ta.CurrentSession(); !ok || s.Active || s.Games != 1 {
		t.Errorf("CurrentSession() = %+v, %v", s, ok)
	}
}

func TestSettingsAndToken(t *testing.T) {
	ta := newTestApp(t, nil)
	var s settingsView
	if code := ta.call("PUT", "/api/settings", `{"visibility":"public","quick_sort_seconds":15,"ballchasing_token":"hijack"}`, &s); code != 200 {
		t.Fatalf("PUT settings status %d", code)
	}
	if s.Visibility != "public" || s.QuickSortSeconds != 15 || s.Token != "" || s.Account.Status != "ok" {
		t.Errorf("settings = %+v", s)
	}
	saved, _ := config.Load(ta.opts.ConfigPath)
	if saved.Visibility != "public" || saved.Token != ballchasingtest.Token {
		t.Errorf("saved settings = %+v", saved)
	}
	var e map[string]string
	if code := ta.call("PUT", "/api/settings", `{"mark_key":"Hyper"}`, &e); code != 400 || e["error"] == "" {
		t.Errorf("invalid key: status %d, %v", code, e)
	}
	if code := ta.call("POST", "/api/token", `{"token":"falsch"}`, &e); code != 400 {
		t.Errorf("wrong token: status %d", code)
	}
	raw, _ := os.ReadFile(ta.opts.ConfigPath)
	if strings.Contains(string(raw), ballchasingtest.Token+`"`) {
		t.Error("token stored in plain text")
	}
}

func TestBorrowAndCleanup(t *testing.T) {
	ta := newTestApp(t, nil)
	id := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private"})
	ta.syncAll(context.Background(), true)
	var res map[string]string
	if code := ta.call("POST", "/api/replays/"+id+"/borrow", "", &res); code != 200 || res["name"] != "bc-"+id+".replay" {
		t.Fatalf("borrow: %d %v", code, res)
	}
	path := filepath.Join(ta.dir, "bc-"+id+".replay")
	if data, _ := os.ReadFile(path); string(data) != "replay-bytes-"+id {
		t.Fatalf("downloaded %q", data)
	}
	ta.cleanBorrows() // left the replay in the game
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("borrowed file not cleaned up")
	}
	ta.Borrow(context.Background(), id)
	ta.KeepBorrow(id, true)
	ta.cleanBorrows()
	if _, err := os.Stat(path); err != nil {
		t.Error("kept file was deleted")
	}
	if _, ok, _ := ta.store.File("bc-" + id + ".replay"); ok {
		t.Error("borrowed file entered the upload queue")
	}
}

func TestGuard(t *testing.T) {
	ta := newTestApp(t, nil)
	resp, _ := http.Get(ta.srv + "/api/state")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("without key: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", ta.srv+"/api/state", nil)
	req.Host = "evil.example:80"
	req.Header.Set("X-Token", ta.token)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign host: %d", resp.StatusCode)
	}
	resp, _ = http.Get(ta.srv + "/")
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), "roMod") || resp.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("index: %d", resp.StatusCode)
	}
	var st stateView
	if ta.call("GET", "/api/state", "", &st); st.Account.Name != "Jamie" || !st.SetupDone {
		t.Errorf("state = %+v", st)
	}
}

func TestDeleteOnlyEmptyFolders(t *testing.T) {
	ta := newTestApp(t, nil)
	full := ta.fake.AddGroup(ballchasingtest.Group{Name: "Voll"})
	empty := ta.fake.AddGroup(ballchasingtest.Group{Name: "Leer"})
	id := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private"})
	ta.API().SetGroup(context.Background(), id, full)
	ta.syncAll(context.Background(), true)
	var e map[string]any
	if code := ta.call("DELETE", "/api/groups/"+full, "", &e); code != http.StatusConflict {
		t.Errorf("deleting a full folder: %d", code)
	}
	if code := ta.call("DELETE", "/api/groups/"+empty, "", &e); code != 200 {
		t.Errorf("deleting an empty folder: %d %v", code, e)
	}
}

func TestEnableStatsAPI(t *testing.T) {
	ta := newTestApp(t, nil)
	var v statsAPIView
	if code := ta.call("POST", "/api/stats-api/enable", "", &v); code != http.StatusBadRequest {
		t.Errorf("without an installation: %d", code)
	}

	ini := filepath.Join(ta.game, "TAGame", "Config", "DefaultStatsAPI.ini")
	os.MkdirAll(filepath.Dir(ini), 0o755)
	os.WriteFile(ini, []byte("[TAGame.MatchStatsExporter_TA]\r\nPort=50000\r\nPacketSendRate=0"), 0o644)
	ta.call("GET", "/api/stats-api", "", &v)
	if len(v.Files) != 1 || v.Files[0].On || v.Files[0].Path != ini {
		t.Fatalf("before: %+v", v)
	}
	if code := ta.call("POST", "/api/stats-api/enable", "", &v); code != 200 {
		t.Fatalf("enable: %d %+v", code, v)
	}
	if len(v.Files) != 1 || !v.Files[0].On || v.Files[0].Port != 50000 || v.Port != 50000 {
		t.Errorf("after: %+v", v)
	}
	if got := ta.config().StatsAPI; got != "127.0.0.1:50000" {
		t.Errorf("the tool connects to %s, not to the game's port", got)
	}
}

func TestAutoEnableStatsAPI(t *testing.T) {
	ta := newTestApp(t, nil)
	ini := filepath.Join(ta.game, "TAGame", "Config", "DefaultStatsAPI.ini")
	os.MkdirAll(filepath.Dir(ini), 0o755)
	off := []byte("[TAGame.MatchStatsExporter_TA]\r\nPort=49123\r\nPacketSendRate=0\r\n")
	os.WriteFile(ini, off, 0o644)
	on := func() bool { s, _ := game.ReadStatsINI(ini); return s.On() }

	ta.autoEnableStatsAPI()
	if on() {
		t.Fatal("switched on without the setting")
	}
	if _, err := ta.UpdateSettings([]byte(`{"stats_api_auto":true}`)); err != nil {
		t.Fatal(err)
	}
	ta.autoEnableStatsAPI() // as at start
	if !on() {
		t.Error("still off at start with the setting")
	}
}

func TestStatsAPIHint(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.AutoMark = false }) // no save key in the match below
	ini := filepath.Join(ta.game, "TAGame", "Config", "TAStatsAPI.ini")
	os.MkdirAll(filepath.Dir(ini), 0o755)
	off := func() {
		os.WriteFile(ini, []byte("[TAGame.MatchStatsExporter_TA]\r\nPacketSendRate=0\r\nPort=1\r\n"), 0o644)
	}
	hints := func() []Toast {
		_, toasts, _ := ta.desk.snapshot()
		return slices.DeleteFunc(toasts, func(t Toast) bool { return t.Kind != "statsapi" })
	}
	// starts the game and lets it run for d without the Stats API
	run := func(d time.Duration) {
		ta.running.Store(false)
		ta.checkStatsAPI()
		ta.running.Store(true)
		ta.checkStatsAPI()
		ta.advance(d)
		ta.checkStatsAPI()
	}
	off()

	run(50 * time.Second)
	if n := len(hints()); n != 0 {
		t.Fatalf("%d hints after 50 seconds", n)
	}
	ta.advance(20 * time.Second)
	ta.tracker.Handle(game.Event{Name: "RoundStarted", MatchGuid: "g"})
	ta.checkStatsAPI()
	if n := len(hints()); n != 0 {
		t.Fatalf("%d hints during a match", n)
	}
	ta.tracker.Handle(game.Event{Name: "MatchDestroyed"})
	ta.checkStatsAPI()
	ta.advance(time.Hour)
	ta.checkStatsAPI()
	h := hints()
	if len(h) != 1 || len(h[0].Actions) != 2 || h[0].Actions[0].Label != "Einschalten" || h[0].Actions[1].Label != "Einstellungen" {
		t.Fatalf("one hint per start of the game with two actions: %+v", h)
	}

	h[0].Actions[1].Run()
	ta.desk.mu.Lock()
	if !slices.Equal(ta.desk.overlay, []string{"einstellungen"}) {
		t.Errorf("overlay: %v", ta.desk.overlay)
	}
	ta.desk.mu.Unlock()
	h[0].Actions[0].Run()
	if s, _ := game.ReadStatsINI(ini); !s.On() || s.Port != 1 {
		t.Errorf("not switched on: %+v", s)
	}
	if h = hints(); len(h) != 2 || h[1].Title != "Rocket League neu starten" {
		t.Errorf("after switching on: %+v", h)
	}

	run(time.Hour)
	if n := len(hints()); n != 2 {
		t.Errorf("hint although the Stats API is on and only the game needs a restart: %d", n)
	}

	off()
	ta.statsAPI.Store(true)
	run(time.Hour)
	ta.statsAPI.Store(false)
	if n := len(hints()); n != 2 {
		t.Errorf("hint although the tool is connected: %d", n)
	}
	run(time.Minute)
	if n := len(hints()); n != 3 {
		t.Errorf("no hint after the next start of the game: %d", n)
	}
}

func TestIgnoreFailedUploads(t *testing.T) {
	ta := newTestApp(t, nil)
	ta.store.PutFile(store.File{Name: "a.replay", State: store.Rejected, Error: "kaputt"})
	ta.store.PutFile(store.File{Name: "b.replay", State: store.Failed, Error: "weg"})
	if ta.queueSummary().Failed != 2 {
		t.Fatal("setup")
	}
	var r map[string]int
	if code := ta.call("POST", "/api/queue/ignore", `{"all":true}`, &r); code != 200 || r["ignored"] != 2 {
		t.Fatalf("ignore all: %d %v", code, r)
	}
	if n := ta.queueSummary().Failed; n != 0 {
		t.Errorf("%d failed after ignoring", n)
	}
	var q struct{ Items []fileView }
	ta.call("GET", "/api/queue", "", &q)
	if len(q.Items) != 2 || q.Items[0].State != store.Ignored {
		t.Errorf("queue: %+v", q.Items)
	}
}

// API returns the test app's client.
func (ta *testApp) API() interface {
	SetGroup(context.Context, string, string) error
} {
	return ta.currentClient()
}
