package game

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

type testTracker struct {
	*Tracker
	results chan error // one result per mark call; blocks while empty
	calls   atomic.Int32
	reminds int
	events  []string // hook calls
}

func newTestTracker(autoMark bool) *testTracker {
	tt := &testTracker{results: make(chan error, 10)}
	tt.Tracker = &Tracker{
		opts:  Options{AutoMark: autoMark, OnlineOnly: true, Key: "Backspace", Hold: time.Millisecond},
		delay: func() time.Duration { return 0 },
		retry: time.Millisecond,
		hooks: Hooks{
			MatchStarted: func() { tt.events = append(tt.events, "start") },
			MatchEnded: func(marked bool) {
				if marked {
					tt.events = append(tt.events, "end marked")
				} else {
					tt.events = append(tt.events, "end")
				}
			},
			ReplayLeft: func() { tt.events = append(tt.events, "replay left") },
		},
		mark: func(ctx context.Context, _ Options) error {
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
	tt.Handle(Event{Name: name, MatchGuid: guid})
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
	tt.results <- ErrNotFocused
	tt.results <- ErrNotFocused
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

func TestHooksAndInMatch(t *testing.T) {
	tt := newTestTracker(true)
	tt.results <- nil

	tt.send("RoundStarted", "G1")
	if !tt.InMatch() {
		t.Error("InMatch() = false during a match")
	}
	waitFor(t, tt.cur.marked.Load)
	tt.send("MatchEnded", "G1")
	if tt.InMatch() {
		t.Error("InMatch() = true after the match")
	}
	tt.send("MatchDestroyed", "G1")
	tt.send("ReplayCreated", "G0")
	tt.send("RoundStarted", "G0")
	tt.send("MatchDestroyed", "G0")

	if want := []string{"start", "end marked", "replay left"}; !slices.Equal(tt.events, want) {
		t.Errorf("hooks = %v, want %v", tt.events, want)
	}
}

func TestPausedNeitherMarksNorReminds(t *testing.T) {
	tt := newTestTracker(true)
	tt.SetPaused(true)
	tt.send("RoundStarted", "G1")
	time.Sleep(20 * time.Millisecond)
	tt.send("MatchEnded", "G1")
	if tt.calls.Load() != 0 || tt.reminds != 0 {
		t.Errorf("paused: %d mark calls, %d reminds, want none", tt.calls.Load(), tt.reminds)
	}

	tt.SetPaused(false)
	tt.SetOptions(Options{AutoMark: false, OnlineOnly: true})
	tt.send("RoundStarted", "G2")
	tt.send("MatchEnded", "G2")
	if tt.calls.Load() != 0 || tt.reminds != 1 {
		t.Errorf("after SetOptions(AutoMark=false): %d calls, %d reminds, want 0 and 1", tt.calls.Load(), tt.reminds)
	}
}

func TestValidKey(t *testing.T) {
	for _, k := range []string{"Backspace", "KeyB", "Digit0", "F5", "F12", "NumpadEnter", "ArrowUp"} {
		if !ValidKey(k) {
			t.Errorf("ValidKey(%q) = false", k)
		}
	}
	if ValidKey("Hyper") || keyCodes["KeyB"].code != 0x30 || keyCodes["KeyQ"].code != 0x10 || keyCodes["F10"].code != 0x44 {
		t.Error("key table wrong")
	}
}
