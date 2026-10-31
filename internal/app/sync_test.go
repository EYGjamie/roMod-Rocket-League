package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing"
	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing/ballchasingtest"
	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
)

func TestDetailsBackOffAndRestWhileTheGameRuns(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.AutoMark = false }) // no save key in the match below
	ctx := context.Background()
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	stuck := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: "Mannfield", Date: base.Add(time.Minute), PendingPolls: 100})
	done := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: "Utopia", Date: base})
	ta.syncAll(ctx, syncNew)
	polls := func() int {
		r, _ := ta.fake.Replay(stuck)
		return r.PendingPolls
	}

	later := map[string]detailRetry{}
	ta.detailStep(ctx, later) // the newer game first: still processing
	if polls() != 99 {
		t.Fatalf("pending polls left %d, want 99", polls())
	}
	ta.detailStep(ctx, later)
	if !ta.hasDetail(done) {
		t.Fatal("the next replay was not fetched")
	}
	ta.detailStep(ctx, later)
	if polls() != 99 {
		t.Fatal("asked again for a replay without statistics right away")
	}

	ta.advance(15 * time.Minute)
	ta.tracker.Handle(game.Event{Name: "RoundStarted", MatchGuid: "g"})
	ta.detailStep(ctx, later)
	if polls() != 99 {
		t.Fatal("fetched statistics during a match")
	}
	ta.tracker.Handle(game.Event{Name: "MatchDestroyed"})
	ta.statsAPI.Store(true) // back in the menu
	ta.detailStep(ctx, later)
	if polls() != 99 {
		t.Fatal("fetched statistics while Rocket League runs")
	}
	ta.statsAPI.Store(false)
	ta.detailStep(ctx, later)
	if polls() != 98 || later[stuck].wait != 30*time.Minute {
		t.Errorf("after 15 minutes: %d polls left, next pause %s", polls(), later[stuck].wait)
	}
}

func TestFolderSyncLooksOnlyWhereSubfoldersCanBe(t *testing.T) {
	ta := newTestApp(t, nil)
	ctx := context.Background()
	if ta.fullSyncDue() {
		t.Error("full sync due right after the one at start")
	}
	ta.advance(25 * time.Hour)
	if !ta.fullSyncDue() {
		t.Error("no full sync due after a day")
	}
	names := func() []string {
		groups, _ := ta.store.Groups()
		var out []string
		for _, g := range groups {
			out = append(out, ta.groupPath(groups, g.ID))
		}
		slices.Sort(out)
		return out
	}
	check := func(when string, want ...string) {
		t.Helper()
		if got := names(); !slices.Equal(got, want) {
			t.Errorf("%s: folders %q, want %q", when, got, want)
		}
	}

	liga := ta.fake.AddGroup(ballchasingtest.Group{Name: "Liga"})
	day := ta.fake.AddGroup(ballchasingtest.Group{Name: "2026-10-01", Parent: liga})
	ta.fake.AddReplay(ballchasingtest.Replay{Group: day})
	ta.syncAll(ctx, syncFull)
	check("new folders", "Liga", "Liga › 2026-10-01")

	ta.fake.AddGroup(ballchasingtest.Group{Name: "leer", Parent: day}) // made on the website
	ta.syncAll(ctx, syncFull)
	check("empty subfolder of a known leaf", "Liga", "Liga › 2026-10-01")
	ta.syncAll(ctx, syncComplete)
	check("complete", "Liga", "Liga › 2026-10-01", "Liga › 2026-10-01 › leer")

	b := ta.fake.AddGroup(ballchasingtest.Group{Name: "B"})
	ta.syncAll(ctx, syncFull)
	kind := ta.fake.AddGroup(ballchasingtest.Group{Name: "Kind", Parent: b})
	ta.fake.AddReplay(ballchasingtest.Replay{Group: kind})
	ta.syncAll(ctx, syncFull)
	check("subfolder with replays", "B", "B › Kind", "Liga", "Liga › 2026-10-01", "Liga › 2026-10-01 › leer")
}

func TestAutoSyncWaitsUntilTheGameCloses(t *testing.T) {
	oldEvery, oldCheck := syncEvery, idleCheck
	syncEvery, idleCheck = 20*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { syncEvery, idleCheck = oldEvery, oldCheck })
	ta := newTestApp(t, nil)
	ta.mu.Lock()
	ta.me = ballchasing.Me{} // as if started while the game runs
	ta.mu.Unlock()
	ta.statsAPI.Store(true)
	has := func(id string) func() bool {
		return func() bool { _, ok, _ := ta.store.Replay(id); return ok }
	}

	first := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles"})
	ta.waitFor("account for the uploads", func() bool { return ta.myAccount().SteamID != "" })
	time.Sleep(100 * time.Millisecond)
	if has(first)() {
		t.Fatal("synced on its own while Rocket League runs")
	}
	ta.requestSync(syncNew) // "Aktualisieren"
	ta.waitFor("sync asked for", has(first))

	second := ta.fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles"})
	time.Sleep(100 * time.Millisecond)
	if has(second)() {
		t.Fatal("synced on its own while Rocket League runs")
	}
	ta.statsAPI.Store(false)
	ta.waitFor("sync after the game closed", has(second))
}

func TestUpdateCheckKeptAcrossRestarts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"tag_name":"v1.1.0","html_url":"https://example.invalid/v1.1.0","assets":[]}`))
	}))
	defer srv.Close()
	old := releaseAPI
	releaseAPI = srv.URL
	t.Cleanup(func() { releaseAPI = old })
	ta := newTestApp(t, nil)
	tag := func() string {
		if rel := ta.release.Load(); rel != nil {
			return rel.Tag
		}
		return ""
	}
	// run runs the update watch of one start of the program for a moment
	run := func(version string) {
		ta.release.Store(nil)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { ta.watchUpdates(ctx, version); close(done) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		<-done
	}

	run("1.0.0")
	if calls.Load() != 1 || tag() != "v1.1.0" {
		t.Fatalf("first start: %d looks, release %q", calls.Load(), tag())
	}
	run("1.0.0")
	if calls.Load() != 1 || tag() != "v1.1.0" {
		t.Errorf("restart: %d looks, release %q, want no new look and the release from before", calls.Load(), tag())
	}
	ta.advance(13 * time.Hour)
	run("1.0.0")
	if calls.Load() != 2 {
		t.Errorf("restart after 13 hours: %d looks, want 2", calls.Load())
	}
	run("1.1.0") // after the update
	if calls.Load() != 3 || tag() != "" {
		t.Errorf("after the update: %d looks, release %q", calls.Load(), tag())
	}
}
