package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"
)

var errNotFocused = errors.New("Rocket League ist nicht im Vordergrund")

// tracker follows matches through Stats API events. On the first kickoff of
// a match it marks the replay for saving; if that did not happen by the end
// of the match it reminds the player to save it by hand.
type tracker struct {
	onlineOnly bool
	autoMark   bool
	delay      func() time.Duration // wait after kickoff before marking
	retry      time.Duration        // wait between attempts while unfocused
	mark       func(context.Context) error
	remind     func()

	inReplay bool
	cur      *match
}

type match struct {
	guid   string
	ended  bool
	cancel context.CancelFunc
	marked atomic.Bool
}

func (t *tracker) handle(ev Event) {
	switch ev.Name {
	case "ReplayCreated":
		t.inReplay = true
	case "MatchDestroyed":
		t.inReplay = false
		t.stop()
		t.cur = nil
	case "RoundStarted":
		t.roundStarted(ev.MatchGuid)
	case "MatchEnded":
		t.matchEnded(ev.MatchGuid)
	}
}

// skip reports whether a match should be ignored: saved replays being
// watched, and offline matches (which have no MatchGuid) if onlineOnly.
func (t *tracker) skip(guid string) bool {
	return t.inReplay || (guid == "" && t.onlineOnly)
}

func (t *tracker) roundStarted(guid string) {
	if t.skip(guid) || (t.cur != nil && t.cur.guid == guid) {
		return
	}
	t.stop()
	ctx, cancel := context.WithCancel(context.Background())
	m := &match{guid: guid, cancel: cancel}
	t.cur = m
	if t.autoMark {
		go t.markLoop(ctx, m)
	}
}

func (t *tracker) matchEnded(guid string) {
	if t.skip(guid) {
		return
	}
	if m := t.cur; m != nil && m.guid == guid {
		if m.ended {
			return
		}
		m.ended = true
		m.cancel()
		if m.marked.Load() {
			return
		}
	}
	log.Print("Match vorbei, Replay ist nicht vorgemerkt: jetzt „Replay speichern“ wählen!")
	t.remind()
}

func (t *tracker) stop() {
	if t.cur != nil {
		t.cur.cancel()
	}
}

func (t *tracker) markLoop(ctx context.Context, m *match) {
	wait := t.delay()
	loggedUnfocused := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		err := t.mark(ctx)
		switch {
		case err == nil:
			m.marked.Store(true)
			log.Print("Backspace gehalten, Replay wird am Matchende gespeichert")
			return
		case ctx.Err() != nil:
			return
		case errors.Is(err, errNotFocused):
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
