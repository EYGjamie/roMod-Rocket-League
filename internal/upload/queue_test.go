package upload

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/EYGjamie/romod/internal/ballchasing"
	"github.com/EYGjamie/romod/internal/ballchasing/ballchasingtest"
	"github.com/EYGjamie/romod/internal/store"
)

type env struct {
	t         *testing.T
	q         *Queue
	st        *store.Store
	fake      *ballchasingtest.Server
	dir       string
	now       time.Time
	processed []string // file names, with " (dup)" for duplicates
	failed    []string
	api       API
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{t: t, st: st, fake: ballchasingtest.New(t), dir: t.TempDir(), now: time.Date(2026, 10, 18, 20, 0, 0, 0, time.Local)}
	e.api = e.fake.API()
	e.q = New(Config{
		Store:      st,
		API:        func() API { return e.api },
		Dirs:       func() []string { return []string{e.dir, filepath.Join(e.dir, "fehlt")} },
		Visibility: func() string { return "unlisted" },
		Processed: func(_ context.Context, f store.File, r ballchasing.Replay, _ []byte, dup bool) {
			name := f.Name
			if dup {
				name += " (dup)"
			}
			if r.Status != "ok" || r.ID != f.ReplayID {
				t.Errorf("Processed(%s) with replay %+v", f.Name, r)
			}
			e.processed = append(e.processed, name)
		},
		Failed: func(f store.File) { e.failed = append(e.failed, f.Name) },
	})
	e.q.now = func() time.Time { return e.now }
	return e
}

func (e *env) write(name, content string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, name), []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) file(name string) store.File {
	e.t.Helper()
	f, ok, err := e.st.File(name)
	if err != nil || !ok {
		e.t.Fatalf("File(%s): %v, %v", name, ok, err)
	}
	return f
}

// drain runs all due steps.
func (e *env) drain() {
	for i := 0; e.q.step(context.Background()); i++ {
		if i > 50 {
			e.t.Fatal("queue does not settle")
		}
	}
}

func TestUploadsSettledNewReplays(t *testing.T) {
	e := newEnv(t)
	e.write("alt.replay", "x")
	if n, err := e.q.Seed(); n != 1 || err != nil {
		t.Fatalf("Seed() = %d, %v", n, err)
	}
	e.write("ranked-doubles_neu.replay", "ab")
	e.write("bc-r9.replay", "geliehen")
	e.write("notiz.txt", "kein Replay")
	e.q.scan() // first sighting
	e.write("ranked-doubles_neu.replay", "abcd")
	e.q.scan() // size still changing
	if _, ok, _ := e.st.File("ranked-doubles_neu.replay"); ok {
		t.Fatal("queued before the file settled")
	}
	e.q.scan()
	if f := e.file("ranked-doubles_neu.replay"); f.State != store.Queued {
		t.Fatalf("state = %s, want queued", f.State)
	}
	e.drain()

	f := e.file("ranked-doubles_neu.replay")
	if f.State != store.Done || f.ReplayID == "" || f.Dir != e.dir {
		t.Errorf("after upload: %+v", f)
	}
	if r, _ := e.fake.Replay(f.ReplayID); r.Visibility != "unlisted" || r.PlaylistID != "ranked-doubles" {
		t.Errorf("uploaded as %+v", r)
	}
	if !slices.Equal(e.processed, []string{"ranked-doubles_neu.replay"}) {
		t.Errorf("processed = %v", e.processed)
	}
	if e.file("alt.replay").State != store.Seeded {
		t.Error("seeded file was touched")
	}
	if _, ok, _ := e.st.File("bc-r9.replay"); ok {
		t.Error("borrowed file was picked up")
	}
}

func TestScanSeesFilesAddedElsewhere(t *testing.T) {
	e := newEnv(t)
	e.write("erst.replay", "x")
	e.q.scan()
	e.q.scan() // the names are cached now
	e.write("hand.replay", "x")
	if err := e.q.AddManual(filepath.Join(e.dir, "hand.replay")); err != nil {
		t.Fatal(err)
	}
	e.q.scan()
	e.q.scan()
	if f := e.file("hand.replay"); !f.Manual {
		t.Errorf("scan replaced a file added by hand: %+v", f)
	}
	e.write("neu.replay", "x")
	e.q.scan()
	e.q.scan()
	if f := e.file("neu.replay"); f.State != store.Queued {
		t.Errorf("new file after the cache was filled: %+v", f)
	}
}

func TestRetriesTwiceThenGivesUp(t *testing.T) {
	e := newEnv(t)
	e.write("boom.replay", "x")
	e.q.AddManual(filepath.Join(e.dir, "boom.replay"))

	e.drain()
	f := e.file("boom.replay")
	if f.State != store.Retry || f.Attempts != 1 || !f.NextTry.Equal(e.now.Add(time.Minute)) || f.Error == "" {
		t.Fatalf("after 1st attempt: %+v", f)
	}
	e.now = e.now.Add(time.Minute)
	e.drain()
	if f = e.file("boom.replay"); f.State != store.Retry || f.Attempts != 2 || !f.NextTry.Equal(e.now.Add(5*time.Minute)) {
		t.Fatalf("after 2nd attempt: %+v", f)
	}
	e.now = e.now.Add(5 * time.Minute)
	e.drain()
	if f = e.file("boom.replay"); f.State != store.Failed || f.Attempts != 3 {
		t.Fatalf("after 3rd attempt: %+v", f)
	}
	e.now = e.now.Add(time.Hour)
	if e.q.step(context.Background()) {
		t.Error("failed file was tried again automatically")
	}
	if !slices.Equal(e.failed, []string{"boom.replay"}) {
		t.Errorf("failed hook = %v", e.failed)
	}

	if err := e.q.Retry("boom.replay"); err != nil {
		t.Fatal(err)
	}
	if f = e.file("boom.replay"); f.State != store.Queued || f.Attempts != 0 || f.Error != "" {
		t.Errorf("after Retry: %+v", f)
	}
}

func TestIgnoredFilesStayAside(t *testing.T) {
	e := newEnv(t)
	e.write("bad.replay", "x") // the fake rejects names starting with "bad"
	e.q.AddManual(filepath.Join(e.dir, "bad.replay"))
	if err := e.q.Ignore("fremd.replay"); err != nil {
		t.Fatal(err)
	}
	e.drain()
	if err := e.q.Ignore("bad.replay"); err != nil {
		t.Fatal(err)
	}
	if f := e.file("bad.replay"); f.State != store.Ignored || f.Error == "" {
		t.Fatalf("after Ignore: %+v", f)
	}
	if _, ok, _ := e.st.File("fremd.replay"); ok {
		t.Error("Ignore added an unknown file")
	}

	// neither scanning nor retrying all brings it back
	e.q.scan()
	e.q.scan()
	if n, _ := e.q.RetryAll(); n != 0 {
		t.Errorf("RetryAll() picked up %d files", n)
	}
	e.now = e.now.Add(time.Hour)
	if e.q.step(context.Background()) {
		t.Error("ignored file was tried again")
	}
	if f := e.file("bad.replay"); f.State != store.Ignored {
		t.Errorf("after scan: %+v", f)
	}

	// a retry by name does
	if err := e.q.Retry("bad.replay"); err != nil {
		t.Fatal(err)
	}
	if f := e.file("bad.replay"); f.State != store.Queued || f.Error != "" {
		t.Errorf("after Retry: %+v", f)
	}
}

func TestOutageAndRejectionDoNotCountAsAttempts(t *testing.T) {
	e := newEnv(t)
	e.write("busy.replay", "x")
	e.write("bad.replay", "x")
	e.q.AddManual(filepath.Join(e.dir, "busy.replay"))
	e.q.AddManual(filepath.Join(e.dir, "bad.replay"))
	e.drain()

	f := e.file("busy.replay")
	if f.State != store.Queued || f.Attempts != 0 || !f.NextTry.Equal(e.now.Add(30*time.Second)) || !e.q.Status().Offline {
		t.Errorf("outage: %+v, status %+v", f, e.q.Status())
	}
	if f := e.file("bad.replay"); f.State != store.Rejected || f.Attempts != 0 {
		t.Errorf("rejected: %+v", f)
	}
	if !slices.Equal(e.failed, []string{"bad.replay"}) {
		t.Errorf("failed hook = %v", e.failed)
	}
	if n, _ := e.q.RetryAll(); n != 0 {
		t.Errorf("RetryAll() picked up %d files, rejected ones need Retry by name", n)
	}
}

func TestBadTokenStopsUntilChanged(t *testing.T) {
	e := newEnv(t)
	good := e.api
	bad := e.fake.API()
	bad.Token = "falsch"
	e.api = bad
	e.write("neu.replay", "x")
	e.q.AddManual(filepath.Join(e.dir, "neu.replay"))
	e.drain()
	if !e.q.Status().BadToken || e.file("neu.replay").State != store.Queued {
		t.Fatalf("status %+v, file %+v", e.q.Status(), e.file("neu.replay"))
	}
	e.api = good
	if e.q.step(context.Background()) {
		t.Error("queue ran on before TokenChanged()")
	}
	e.q.TokenChanged()
	e.drain()
	if e.file("neu.replay").State != store.Done {
		t.Error("upload did not resume after TokenChanged()")
	}
}

func TestWaitsForProcessingAndHandlesDuplicates(t *testing.T) {
	e := newEnv(t)
	tmpl := e.fake.Template
	e.fake.Template = func(file string) ballchasingtest.Replay {
		r := tmpl(file)
		r.PendingPolls = 2
		return r
	}
	e.write("neu.replay", "x")
	e.q.AddManual(filepath.Join(e.dir, "neu.replay"))
	e.drain() // upload and first poll
	if f := e.file("neu.replay"); f.State != store.Processing || !f.NextTry.Equal(e.now.Add(3*time.Second)) {
		t.Fatalf("while pending: %+v", f)
	}
	for range 2 {
		e.now = e.now.Add(3 * time.Second)
		e.drain()
	}
	if f := e.file("neu.replay"); f.State != store.Done {
		t.Fatalf("after processing: %+v", f)
	}

	// the same file again, e.g. after deleting the state
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "neu.replay"), []byte("x"), 0o644)
	e.st.PutFile(store.File{Name: "neu.replay", Dir: other, State: store.Queued})
	e.drain()
	if want := []string{"neu.replay", "neu.replay (dup)"}; !slices.Equal(e.processed, want) {
		t.Errorf("processed = %v, want %v", e.processed, want)
	}
}

func TestPauseMissingFileAndOldReplays(t *testing.T) {
	e := newEnv(t)
	e.write("alt1.replay", "x")
	e.write("alt2.replay", "x")
	old := e.now.Add(-60 * 24 * time.Hour)
	os.Chtimes(filepath.Join(e.dir, "alt1.replay"), old, old)
	e.q.Seed()

	if n := e.q.CountSeeded(e.now.Add(-30 * 24 * time.Hour)); n != 1 {
		t.Errorf("CountSeeded(30 days) = %d, want 1", n)
	}
	e.q.SetPaused(true)
	e.write("neu.replay", "x")
	e.q.scan()
	e.q.scan()
	if _, ok, _ := e.st.File("neu.replay"); ok {
		t.Error("paused queue picked up a new file")
	}
	if n, _ := e.q.QueueSeeded(time.Time{}); n != 2 {
		t.Errorf("QueueSeeded(all) = %d, want 2", n)
	}
	if e.q.step(context.Background()) {
		t.Error("paused queue uploaded")
	}
	e.q.SetPaused(false)
	os.Remove(filepath.Join(e.dir, "alt2.replay"))
	e.drain()
	if f := e.file("alt1.replay"); f.State != store.Done {
		t.Errorf("alt1: %+v", f)
	}
	if f := e.file("alt2.replay"); f.State != store.Failed || f.Error != "Datei nicht mehr vorhanden" {
		t.Errorf("alt2: %+v", f)
	}
	e.q.scan()
	e.q.scan()
	if f := e.file("neu.replay"); f.State != store.Queued {
		t.Errorf("file saved during the pause: %+v", f)
	}
}
