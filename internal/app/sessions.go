package app

import (
	"fmt"
	"sort"
	"time"

	"github.com/EYGjamie/romod/internal/rules"
	"github.com/EYGjamie/romod/internal/store"
)

const sessionGap = 30 * time.Minute

// Session is a run of games with at most 30 minutes between two of them.
type Session struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Games   int       `json:"games"`
	Wins    int       `json:"wins"`
	Losses  int       `json:"losses"`
	Results []string  `json:"results"` // oldest first: win, loss or ""
	Streak  int       `json:"streak"`  // longest run of wins
	IDs     []string  `json:"ids"`
	Active  bool      `json:"active"`
}

// visible returns the replays without hidden duplicates, newest first.
func (a *App) visible() []store.Replay {
	all, _ := a.store.Replays()
	out := all[:0]
	for _, r := range all {
		if r.DupOf == "" {
			out = append(out, r)
		}
	}
	return out
}

// mine reports whether the user played in r. Without known accounts, own
// uploads count.
func mine(r store.Replay, me map[string]bool) bool {
	if len(me) == 0 {
		return r.Own
	}
	return r.MyTeam(me) != ""
}

// sessionsOf groups the user's games into sessions, oldest first.
func sessionsOf(replays []store.Replay, me map[string]bool) []Session {
	var games []store.Replay
	for _, r := range replays {
		if mine(r, me) && !r.Date.IsZero() {
			games = append(games, r)
		}
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Date.Before(games[j].Date) })
	var out []Session
	for _, r := range games {
		if n := len(out); n == 0 || r.Date.Sub(out[n-1].End) > sessionGap {
			out = append(out, Session{Start: r.Date})
		}
		s := &out[len(out)-1]
		s.End = r.Date
		s.Games++
		res := r.Result(me)
		s.Results = append(s.Results, res)
		s.IDs = append(s.IDs, r.ID)
		switch res {
		case "win":
			s.Wins++
		case "loss":
			s.Losses++
		}
	}
	for i := range out {
		run := 0
		for _, res := range out[i].Results {
			if res == "win" {
				run++
				out[i].Streak = max(out[i].Streak, run)
			} else {
				run = 0
			}
		}
	}
	return out
}

// CurrentSession returns the latest session; Active means it may go on.
func (a *App) CurrentSession() (Session, bool) {
	sessions := sessionsOf(a.visible(), a.myIDs())
	if len(sessions) == 0 {
		return Session{}, false
	}
	s := sessions[len(sessions)-1]
	a.mu.RLock()
	last := a.lastGame
	a.mu.RUnlock()
	end := s.End
	if last.After(end) {
		end = last
	}
	s.Active = a.now().Sub(end) < sessionGap
	return s, true
}

// checkSessionEnd announces a session that ended while the app ran: 30
// minutes after the last game, or right away when the game was closed.
func (a *App) checkSessionEnd(gameClosed bool) {
	s, ok := a.CurrentSession()
	if !ok || s.Games == 0 {
		return
	}
	a.mu.Lock()
	if !s.End.After(a.notified) || s.Active && !gameClosed {
		a.mu.Unlock()
		return
	}
	a.notified = s.End
	a.mu.Unlock()
	if !a.config().NotifySession {
		return
	}
	body := fmt.Sprintf("%d Spiele, %d–%d", s.Games, s.Wins, s.Losses)
	if s.Streak > 1 {
		body += fmt.Sprintf(", längste Siegesfolge %d", s.Streak)
	}
	a.toast(Toast{Kind: "session", Title: "Session beendet", Body: body,
		Actions: []ToastAction{
			{Label: "Session einordnen", Run: func() { a.desk.ShowOverlay("eingang") }},
			{Label: "Statistik", Run: func() { a.desk.ShowOverlay("statistik") }},
		},
		Disable: func() { a.UpdateSettings([]byte(`{"notify_session":false}`)) }})
}

// Stats is the statistics page for a period.
type Stats struct {
	Days      int       `json:"days"` // 0 = all time
	Unit      string    `json:"unit"` // day or month
	Uploads   int       `json:"uploads"`
	Games     int       `json:"games"`
	Wins      int       `json:"wins"`
	Losses    int       `json:"losses"`
	PerPeriod []Period  `json:"per_period"`
	Playlists []Tally   `json:"playlists"`
	Maps      []Tally   `json:"maps"` // best maps first, at least 3 games
	Since     time.Time `json:"since"`
}

type Period struct {
	Label  string    `json:"label"`
	Start  time.Time `json:"start"`
	Wins   int       `json:"wins"`
	Losses int       `json:"losses"`
	Other  int       `json:"other"`
}

type Tally struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Games  int    `json:"games"`
	Wins   int    `json:"wins"`
	Losses int    `json:"losses"`
}

// Stats computes the statistics of the last days (all time if 0).
func (a *App) Stats(days int) Stats {
	me := a.myIDs()
	now := a.now()
	st := Stats{Days: days, Unit: "day"}
	day := func(t time.Time) time.Time { y, m, d := t.Date(); return time.Date(y, m, d, 0, 0, 0, 0, t.Location()) }
	month := func(t time.Time) time.Time { y, m, _ := t.Date(); return time.Date(y, m, 1, 0, 0, 0, 0, t.Location()) }
	if days > 0 {
		st.Since = day(now).AddDate(0, 0, -(days - 1))
	}
	replays := a.visible()
	if days == 0 {
		st.Unit = "month"
		st.Since = month(now)
		for _, r := range replays {
			if mine(r, me) && !r.Date.IsZero() && r.Date.Before(st.Since) {
				st.Since = month(r.Date)
			}
		}
	}
	periods := map[time.Time]*Period{}
	var order []time.Time
	for t := st.Since; !t.After(now); {
		p := &Period{Start: t}
		if st.Unit == "day" {
			p.Label = t.Format("02.01.")
			t = t.AddDate(0, 0, 1)
		} else {
			p.Label = t.Format("01/2006")
			t = t.AddDate(0, 1, 0)
		}
		periods[p.Start] = p
		order = append(order, p.Start)
	}
	playlists := map[string]*Tally{}
	maps := map[string]*Tally{}
	for _, r := range replays {
		if r.Own && !r.Created.Before(st.Since) {
			st.Uploads++
		}
		if !mine(r, me) || r.Date.Before(st.Since) {
			continue
		}
		res := r.Result(me)
		st.Games++
		key := day(r.Date)
		if st.Unit == "month" {
			key = month(r.Date)
		}
		p := periods[key]
		id := rules.Canonical(r.PlaylistID)
		pl := playlists[id]
		if pl == nil {
			pl = &Tally{ID: id, Name: rules.PlaylistName(id)}
			playlists[id] = pl
		}
		mp := maps[r.MapName]
		if mp == nil {
			mp = &Tally{ID: r.MapCode, Name: r.MapName}
			maps[r.MapName] = mp
		}
		pl.Games++
		mp.Games++
		switch res {
		case "win":
			st.Wins++
			pl.Wins++
			mp.Wins++
			if p != nil {
				p.Wins++
			}
		case "loss":
			st.Losses++
			pl.Losses++
			mp.Losses++
			if p != nil {
				p.Losses++
			}
		default:
			if p != nil {
				p.Other++
			}
		}
	}
	for _, t := range order {
		st.PerPeriod = append(st.PerPeriod, *periods[t])
	}
	for _, t := range playlists {
		st.Playlists = append(st.Playlists, *t)
	}
	sort.Slice(st.Playlists, func(i, j int) bool {
		if st.Playlists[i].Games != st.Playlists[j].Games {
			return st.Playlists[i].Games > st.Playlists[j].Games
		}
		return st.Playlists[i].Name < st.Playlists[j].Name
	})
	for _, t := range maps {
		if t.Games >= 3 {
			st.Maps = append(st.Maps, *t)
		}
	}
	rate := func(t Tally) float64 { return float64(t.Wins) / float64(t.Games) }
	sort.Slice(st.Maps, func(i, j int) bool {
		if rate(st.Maps[i]) != rate(st.Maps[j]) {
			return rate(st.Maps[i]) > rate(st.Maps[j])
		}
		return st.Maps[i].Games > st.Maps[j].Games
	})
	st.Maps = st.Maps[:min(len(st.Maps), 8)]
	return st
}
