package ballchasing_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing/ballchasingtest"
)

func writeReplay(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("replay-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUpload(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()

	id, dup, err := bc.Upload(ctx, writeReplay(t, "ranked-duels_neu.replay"), "unlisted")
	if err != nil || id == "" || dup {
		t.Fatalf("Upload() = %q, %v, %v", id, dup, err)
	}
	again, dup, err := bc.Upload(ctx, writeReplay(t, "ranked-duels_neu.replay"), "unlisted")
	if err != nil || again != id || !dup {
		t.Errorf("second Upload() = %q, %v, %v, want %q as duplicate", again, dup, err, id)
	}

	if _, _, err := bc.Upload(ctx, writeReplay(t, "bad.replay"), "private"); !errors.Is(err, ballchasing.ErrRejected) {
		t.Errorf("bad replay: error = %v, want ErrRejected", err)
	}
	_, _, err = bc.Upload(ctx, writeReplay(t, "busy.replay"), "private")
	if errors.Is(err, ballchasing.ErrRejected) || !ballchasing.Unavailable(err) {
		t.Errorf("busy server: error = %v, want retryable and unavailable", err)
	}
	_, _, err = bc.Upload(ctx, writeReplay(t, "boom.replay"), "private")
	if err == nil || ballchasing.Unavailable(err) {
		t.Errorf("server error: error = %v, want a counted failure", err)
	}
	if _, _, err := bc.Upload(ctx, filepath.Join(t.TempDir(), "fehlt.replay"), "private"); err == nil || ballchasing.Unavailable(err) {
		t.Errorf("missing file: error = %v, must not look like an outage", err)
	}
}

func TestGetReplayWaitsForProcessing(t *testing.T) {
	fake := ballchasingtest.New(t)
	bc := fake.API()
	ctx := context.Background()
	id := fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Map: "Mannfield", PendingPolls: 1,
		Blue:   []ballchasingtest.Player{{Name: "Jamie", PID: "steam:1", Goals: 2, Shots: 3}},
		Orange: []ballchasingtest.Player{{Name: "Gegner", PID: "epic:x", Goals: 1}}})

	if r, _, err := bc.GetReplay(ctx, id); err != nil || r.Status != "pending" {
		t.Fatalf("first GetReplay() = %q, %v, want pending", r.Status, err)
	}
	r, raw, err := bc.GetReplay(ctx, id)
	if err != nil || r.Status != "ok" {
		t.Fatalf("GetReplay() = %q, %v", r.Status, err)
	}
	if r.PlaylistID != "ranked-doubles" || r.MatchGUID == "" || r.Blue.TotalGoals() != 2 || r.Orange.TotalGoals() != 1 {
		t.Errorf("replay decoded as %+v", r)
	}
	p := r.Blue.Players[0]
	if p.PID() != "steam:1" || p.Stats == nil || p.Stats.Core.Shots != 3 {
		t.Errorf("player decoded as %+v", p)
	}
	if !bytes.Contains(raw, []byte(`"possession_time"`)) {
		t.Error("raw reply misses the statistics")
	}
}

func TestPing(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()

	if me, err := bc.Ping(ctx); err != nil || me.Name != "Jamie" || me.SteamID == "" {
		t.Errorf("Ping() = %+v, %v", me, err)
	}
	bc.Token = "falsch"
	if _, err := bc.Ping(ctx); !errors.Is(err, ballchasing.ErrBadToken) {
		t.Errorf("Ping() error = %v, want ErrBadToken", err)
	}
}

func TestListReplays(t *testing.T) {
	fake := ballchasingtest.New(t)
	bc := fake.API()
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	for i := range 3 {
		fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private", Created: base.Add(time.Duration(i) * time.Hour),
			Blue: []ballchasingtest.Player{{Name: "Jamie", PID: "steam:1"}}})
	}
	fake.AddReplay(ballchasingtest.Replay{PlaylistID: "ranked-doubles", Created: base.Add(5 * time.Hour), UploaderID: "999", UploaderName: "Max",
		Visibility: "public", Blue: []ballchasingtest.Player{{Name: "Max", PID: "steam:999"}, {Name: "Jamie", PID: "steam:1"}}})

	page, err := bc.ListReplays(ctx, ballchasing.ReplayQuery{Uploader: "me", Count: 2, Oldest: true}, "")
	if err != nil || len(page.List) != 2 || page.Next == "" || page.List[0].Name() == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	if page.List[0].Created > page.List[1].Created {
		t.Error("oldest uploads should come first")
	}
	page, err = bc.ListReplays(ctx, ballchasing.ReplayQuery{}, page.Next)
	if err != nil || len(page.List) != 1 || page.Next != "" {
		t.Errorf("second page = %+v, %v", page, err)
	}

	page, err = bc.ListReplays(ctx, ballchasing.ReplayQuery{PlayerID: "steam:1", CreatedAfter: base.Add(90 * time.Minute)}, "")
	if err != nil || len(page.List) != 2 {
		t.Errorf("player-id since 21:30 = %d replays, %v, want 2", len(page.List), err)
	}

	if _, err := bc.ListReplays(ctx, ballchasing.ReplayQuery{}, "https://example.com/api/replays?x=1"); err == nil {
		t.Error("ListReplays() followed a foreign next link")
	}
}

func TestGroupsAndReplayChanges(t *testing.T) {
	fake := ballchasingtest.New(t)
	bc := fake.API()
	ctx := context.Background()
	liga := fake.AddGroup(ballchasingtest.Group{Name: "Liga"})
	fake.AddGroup(ballchasingtest.Group{Name: "Spieltag 1", Parent: liga})
	id := fake.AddReplay(ballchasingtest.Replay{PlaylistID: "private"})

	if top, err := bc.ListGroups(ctx, ""); err != nil || len(top) != 1 || top[0].Name != "Liga" {
		t.Errorf("top-level groups = %+v, %v", top, err)
	}
	if sub, err := bc.ListGroups(ctx, liga); err != nil || len(sub) != 1 || sub[0].Name != "Spieltag 1" {
		t.Errorf("subgroups = %+v, %v", sub, err)
	}
	sub, err := bc.CreateGroup(ctx, "Spieltag 2", liga, true)
	if err != nil || sub == "" {
		t.Fatalf("CreateGroup() below Liga = %q, %v", sub, err)
	}
	if _, err := bc.CreateGroup(ctx, "Spieltag 3", "weg", false); err == nil {
		t.Error("CreateGroup() below a missing parent succeeded")
	}

	if err := bc.SetGroup(ctx, id, sub); err != nil {
		t.Fatal(err)
	}
	if err := bc.SetVisibility(ctx, id, "public"); err != nil {
		t.Fatal(err)
	}
	if r, _ := fake.Replay(id); r.Group != sub || r.Visibility != "public" {
		t.Errorf("replay after changes = group %q, visibility %q", r.Group, r.Visibility)
	}
	if groups, _ := bc.ListGroups(ctx, liga); len(groups) != 2 {
		t.Errorf("Liga has %d subgroups, want 2", len(groups))
	}
	if top, _ := bc.ListGroups(ctx, ""); top[0].IndirectReplays != 1 || top[0].DirectReplays != 0 {
		t.Errorf("Liga counts = %d direct, %d with subgroups, want 0 and 1", top[0].DirectReplays, top[0].IndirectReplays)
	}

	var file bytes.Buffer
	if err := bc.Download(ctx, id, &file); err != nil || file.String() != "replay-bytes-"+id {
		t.Errorf("Download() = %q, %v", file.String(), err)
	}
	if err := bc.DeleteReplay(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Replay(id); ok {
		t.Error("replay still there after DeleteReplay()")
	}
}

func TestRateLimitIsRetried(t *testing.T) {
	fake := ballchasingtest.New(t)
	bc := fake.API()
	fake.AddReplay(ballchasingtest.Replay{ID: "r-limit", PlaylistID: "private"})
	g := fake.AddGroup(ballchasingtest.Group{Name: "Liga"})
	if err := bc.SetGroup(context.Background(), "r-limit", g); err != nil {
		t.Fatalf("SetGroup() after 429 = %v, want retried", err)
	}
	if got := fake.Patched("r-limit"); len(got) != 1 {
		t.Errorf("patched %v, want one successful retry", got)
	}
}
