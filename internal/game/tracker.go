package game

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNotFocused means the key was not sent because Rocket League was not
// the active window.
var ErrNotFocused = errors.New("Rocket League ist nicht im Vordergrund")

// Options control marking replays; they can change while the tracker runs.
type Options struct {
	AutoMark   bool          // hold the save key after the first kickoff
	OnlineOnly bool          // ignore offline matches and training
	Key        string        // KeyboardEvent.code of the save key
	Hold       time.Duration // how long the key is held
}

// Hooks tell the rest of the app about matches. Each may be nil.
type Hooks struct {
	MatchStarted func()            // first kickoff of a tracked match
	MatchEnded   func(marked bool) // end of a tracked match
	ReplayLeft   func()            // the player stopped watching a saved replay

	// Lobby receives the players of an online match from every UpdateState,
	// Winner the team that won it (0 blue, 1 orange); not for replays.
	Lobby  func(guid string, players []Player)
	Winner func(guid string, team int)
}

// Tracker follows matches through Stats API events. On the first kickoff of
// a match it marks the replay for saving; if that did not happen by the end
// of the match it reminds the player to save it by hand.
type Tracker struct {
	hooks  Hooks
	delay  func() time.Duration // wait after kickoff before marking
	retry  time.Duration        // wait between attempts while unfocused
	mark   func(context.Context, Options) error
	remind func()

	mu     sync.Mutex
	opts   Options
	paused bool

	inMatch  atomic.Bool
	inReplay bool
	cur      *match
}

// NewTracker returns a tracker that marks 3–8 seconds after the first
// kickoff and beeps when a match ends without a marked replay.
func NewTracker(opts Options, hooks Hooks) *Tracker {
	return &Tracker{
		opts:   opts,
		hooks:  hooks,
		delay:  func() time.Duration { return 3*time.Second + rand.N(5*time.Second) },
		retry:  3 * time.Second,
		mark:   func(ctx context.Context, o Options) error { return markReplay(ctx, o.Key, o.Hold) },
		remind: beep,
	}
}

// SetOptions applies changed settings from the next match on.
func (t *Tracker) SetOptions(o Options) {
	t.mu.Lock()
	t.opts = o
	t.mu.Unlock()
}

// SetPaused stops or resumes marking and reminding.
func (t *Tracker) SetPaused(p bool) {
	t.mu.Lock()
	t.paused = p
	t.mu.Unlock()
}

func (t *Tracker) state() (Options, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.opts, t.paused
}

// InMatch reports whether a tracked match is running.
func (t *Tracker) InMatch() bool { return t.inMatch.Load() }

type match struct {
	guid   string
	ended  bool
	cancel context.CancelFunc
	marked atomic.Bool
}

// Handle processes one Stats API event. Calls must not overlap.
func (t *Tracker) Handle(ev Event) {
	switch ev.Name {
	case "ReplayCreated":
		t.inReplay = true
	case "MatchDestroyed":
		left := t.inReplay
		t.inReplay = false
		t.inMatch.Store(false)
		t.stop()
		t.cur = nil
		if left && t.hooks.ReplayLeft != nil {
			t.hooks.ReplayLeft()
		}
	case "RoundStarted":
		t.roundStarted(ev.MatchGuid)
	case "MatchEnded":
		t.matchEnded(ev.MatchGuid)
		if t.hooks.Winner != nil && ev.MatchGuid != "" && ev.Winner >= 0 && !t.inReplay {
			t.hooks.Winner(ev.MatchGuid, ev.Winner)
		}
	case "UpdateState":
		if t.hooks.Lobby != nil && ev.MatchGuid != "" && !t.inReplay {
			t.hooks.Lobby(ev.MatchGuid, ev.Players)
		}
	}
}

// skip reports whether a match should be ignored: saved replays being
// watched, and offline matches (which have no MatchGuid) if OnlineOnly.
func (t *Tracker) skip(guid string) bool {
	opts, _ := t.state()
	return t.inReplay || (guid == "" && opts.OnlineOnly)
}

func (t *Tracker) roundStarted(guid string) {
	if t.skip(guid) || (t.cur != nil && t.cur.guid == guid) {
		return
	}
	t.stop()
	ctx, cancel := context.WithCancel(context.Background())
	m := &match{guid: guid, cancel: cancel}
	t.cur = m
	t.inMatch.Store(true)
	if t.hooks.MatchStarted != nil {
		t.hooks.MatchStarted()
	}
	if opts, paused := t.state(); opts.AutoMark && !paused {
		go t.markLoop(ctx, m)
	}
}

func (t *Tracker) matchEnded(guid string) {
	if t.skip(guid) {
		return
	}
	t.inMatch.Store(false)
	marked := false
	if m := t.cur; m != nil && m.guid == guid {
		if m.ended {
			return
		}
		m.ended = true
		m.cancel()
		marked = m.marked.Load()
	}
	if t.hooks.MatchEnded != nil {
		t.hooks.MatchEnded(marked)
	}
	if _, paused := t.state(); marked || paused {
		return
	}
	log.Print("Match vorbei, Replay ist nicht vorgemerkt: jetzt „Replay speichern“ wählen!")
	t.remind()
}

func (t *Tracker) stop() {
	if t.cur != nil {
		t.cur.cancel()
	}
}

func (t *Tracker) markLoop(ctx context.Context, m *match) {
	wait := t.delay()
	loggedUnfocused := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		opts, paused := t.state()
		if paused {
			return
		}
		err := t.mark(ctx, opts)
		switch {
		case err == nil:
			m.marked.Store(true)
			log.Print("Speichern-Taste gehalten, Replay wird am Matchende gespeichert")
			return
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrNotFocused):
			if !loggedUnfocused {
				log.Print("Rocket League ist nicht im Vordergrund, versuche es weiter …")
				loggedUnfocused = true
			}
		default:
			log.Printf("Vormerken fehlgeschlagen: %v", err)
			return
		}
		wait = t.retry
	}
}

// PressSaveKey holds the save key once if Rocket League is in front, for
// trying the key from the settings.
func PressSaveKey(ctx context.Context, key string, hold time.Duration) error {
	return markReplay(ctx, key, hold)
}
