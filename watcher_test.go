package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherUploadsSettledNewReplays(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "uploaded.json")
	state, existed, err := loadState(statePath)
	if err != nil || existed {
		t.Fatalf("loadState() = %v, %v", existed, err)
	}
	writeFile(t, filepath.Join(dir, "old.replay"), "x")

	var uploaded []string
	w := newWatcher([]string{dir, filepath.Join(dir, "missing")}, state, func(_ context.Context, path string) (string, bool, error) {
		uploaded = append(uploaded, filepath.Base(path))
		return "id-" + filepath.Base(path), false, nil
	})
	if n, err := w.seed(); n != 1 || err != nil {
		t.Fatalf("seed() = %d, %v, want 1, nil", n, err)
	}

	newPath := filepath.Join(dir, "new.replay")
	writeFile(t, newPath, "ab")
	writeFile(t, filepath.Join(dir, "notes.txt"), "kein Replay")
	w.scan(ctx) // first sighting
	writeFile(t, newPath, "abcd")
	w.scan(ctx) // size still changing
	if len(uploaded) != 0 {
		t.Fatalf("uploaded %v before the file settled", uploaded)
	}
	w.scan(ctx)
	w.scan(ctx)
	if !slices.Equal(uploaded, []string{"new.replay"}) {
		t.Fatalf("uploaded = %v, want [new.replay]", uploaded)
	}

	saved, existed, err := loadState(statePath)
	if err != nil || !existed {
		t.Fatalf("loadState() = %v, %v", existed, err)
	}
	if saved.Uploaded["new.replay"] != "id-new.replay" {
		t.Errorf("saved ID = %q, want id-new.replay", saved.Uploaded["new.replay"])
	}
	if id, ok := saved.Uploaded["old.replay"]; !ok || id != "" {
		t.Errorf("old.replay = %q, %v, want seeded as skipped", id, ok)
	}
}

func TestWatcherBacksOffAndSkipsRejected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	state, _, err := loadState(filepath.Join(t.TempDir(), "uploaded.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.replay"), "a")
	writeFile(t, filepath.Join(dir, "b.replay"), "b")

	calls := map[string]int{}
	w := newWatcher([]string{dir}, state, func(_ context.Context, path string) (string, bool, error) {
		name := filepath.Base(path)
		calls[name]++
		if name == "b.replay" {
			return "", false, errRejected
		}
		if calls[name] == 1 {
			return "", false, errors.New("Netzwerk weg")
		}
		return "id-a", false, nil
	})
	now := time.Unix(0, 0)
	w.now = func() time.Time { return now }

	w.scan(ctx)
	w.scan(ctx) // a fails, b rejected
	w.scan(ctx) // a still backing off
	if calls["a.replay"] != 1 || calls["b.replay"] != 1 {
		t.Fatalf("calls = %v, want one each", calls)
	}
	if _, ok := state.Uploaded["b.replay"]; !ok {
		t.Error("rejected b.replay not marked as handled")
	}

	now = now.Add(31 * time.Second)
	w.scan(ctx)
	if calls["a.replay"] != 2 || state.Uploaded["a.replay"] != "id-a" {
		t.Errorf("calls = %v, state = %v, want a uploaded on retry", calls, state.Uploaded)
	}
}
