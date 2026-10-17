package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type testTracker struct {
	*tracker
	results chan error // one result per mark call; blocks while empty
	calls   atomic.Int32
	reminds int
}

func newTestTracker(autoMark bool) *testTracker {
	tt := &testTracker{results: make(chan error, 10)}
	tt.tracker = &tracker{
		onlineOnly: true,
		autoMark:   autoMark,
		delay:      func() time.Duration { return 0 },
		retry:      time.Millisecond,
		mark: func(ctx context.Context) error {
			tt.calls.Add(1)
			select {
			case err := <-tt.results:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		remind: func() { tt.reminds++ },
	}
	return tt
}

func (tt *testTracker) send(name, guid string) {
	tt.handle(Event{Name: name, MatchGuid: guid})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("Bedingung nicht rechtzeitig erfüllt")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestMarksOncePerMatch(t *testing.T) {
	tt := newTestTracker(true)
	tt.results <- nil

	tt.send("RoundStarted", "G1")
	m := tt.cur
	waitFor(t, m.marked.Load)
	tt.send("RoundStarted", "G1") // kickoff after a goal
	time.Sleep(20 * time.Millisecond)
	tt.send("MatchEnded", "G1")

	if n := tt.calls.Load(); n != 1 {
		t.Errorf("mark calls = %d, want 1", n)
	}
	if tt.reminds != 0 {
		t.Errorf("reminds = %d, want 0", tt.reminds)
	}
}

func TestRetriesWhileUnfocused(t *testing.T) {
	tt := newTestTracker(true)
	tt.results <- errNotFocused
	tt.results <- errNotFocused
	tt.results <- nil

	tt.send("RoundStarted", "G1")
	waitFor(t, tt.cur.marked.Load)

	if n := tt.calls.Load(); n != 3 {
		t.Errorf("mark calls = %d, want 3", n)
	}
}

func TestRemindsWhenNotMarked(t *testing.T) {
	tt := newTestTracker(true) // mark blocks until the match ends

	tt.send("RoundStarted", "G1")
	waitFor(t, func() bool { return tt.calls.Load() == 1 })
	tt.send("MatchEnded", "G1")
	tt.send("MatchEnded", "G1")

	if tt.reminds != 1 {
		t.Errorf("reminds = %d, want 1", tt.reminds)
	}
	if tt.cur.marked.Load() {
		t.Error("match marked although mark never succeeded")
	}
}

func TestSkipsOfflineMatchesAndSavedReplays(t *testing.T) {
	tt := newTestTracker(true)
	tt.results <- nil

	tt.send("RoundStarted", "") // offline match
	tt.send("MatchEnded", "")
	tt.send("MatchDestroyed", "")

	tt.send("ReplayCreated", "G0") // watching a saved replay
	tt.send("RoundStarted", "G0")
	tt.send("MatchEnded", "G0")
	tt.send("MatchDestroyed", "G0")

	if n := tt.calls.Load(); n != 0 {
		t.Fatalf("mark calls = %d, want 0", n)
	}
	if tt.reminds != 0 {
		t.Fatalf("reminds = %d, want 0", tt.reminds)
	}

	tt.send("RoundStarted", "G1")
	waitFor(t, tt.cur.marked.Load)
}

func TestReminderOnlyMode(t *testing.T) {
	tt := newTestTracker(false)

	tt.send("RoundStarted", "G1")
	tt.send("MatchEnded", "G1")

	if n := tt.calls.Load(); n != 0 {
		t.Errorf("mark calls = %d, want 0", n)
	}
	if tt.reminds != 1 {
		t.Errorf("reminds = %d, want 1", tt.reminds)
	}
}
