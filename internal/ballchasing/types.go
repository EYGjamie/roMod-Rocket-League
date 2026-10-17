package ballchasing

import (
	"context"
	"sync"
	"time"
)

// Replay holds the fields of a replay this tool works with. The list
// endpoint fills the summary, GET /replays/{id} also the statistics.
type Replay struct {
	ID             string `json:"id"`
	RocketLeagueID string `json:"rocket_league_id"`
	MatchGUID      string `json:"match_guid"`
	Title          string `json:"title"`
	ReplayTitle    string `json:"replay_title"` // the list endpoint's name for Title
	Status         string `json:"status"`       // ok, pending or failed
	Created        string `json:"created"`
	Date           string `json:"date"`
	Uploader       struct {
		SteamID string `json:"steam_id"`
		Name    string `json:"name"`
	} `json:"uploader"`
	Visibility   string     `json:"visibility"`
	PlaylistID   string     `json:"playlist_id"`
	PlaylistName string     `json:"playlist_name"`
	MapCode      string     `json:"map_code"`
	MapName      string     `json:"map_name"`
	Duration     float64    `json:"duration"`
	Overtime     bool       `json:"overtime"`
	Groups       []GroupRef `json:"groups"`
	Blue         Team       `json:"blue"`
	Orange       Team       `json:"orange"`
}

// Name returns the replay title from whichever endpoint filled it.
func (r Replay) Name() string {
	if r.Title != "" {
		return r.Title
	}
	return r.ReplayTitle
}

type Team struct {
	Name  string `json:"name"`
	Goals int    `json:"goals"` // list endpoint
	Stats struct {
		Core struct {
			Goals int `json:"goals"`
		} `json:"core"`
	} `json:"stats"` // replay endpoint
	Players []Player `json:"players"`
}

// TotalGoals returns the team's goals from either endpoint.
func (t Team) TotalGoals() int {
	return max(t.Goals, t.Stats.Core.Goals)
}

type Player struct {
	Name string `json:"name"`
	ID   struct {
		Platform string `json:"platform"`
		ID       string `json:"id"`
	} `json:"id"`
	Score int  `json:"score"` // list endpoint
	MVP   bool `json:"mvp"`
	Stats *struct {
		Core struct {
			Score   int  `json:"score"`
			Goals   int  `json:"goals"`
			Assists int  `json:"assists"`
			Saves   int  `json:"saves"`
			Shots   int  `json:"shots"`
			MVP     bool `json:"mvp"`
		} `json:"core"`
	} `json:"stats"` // replay endpoint
}

// PID identifies a player across replays, e.g. "steam:76561198…".
func (p Player) PID() string {
	if p.ID.ID == "" {
		return ""
	}
	return p.ID.Platform + ":" + p.ID.ID
}

type GroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ReplayPage struct {
	List []Replay `json:"list"`
	Next string   `json:"next,omitempty"`
}

type Group struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Created         string `json:"created"`
	DirectReplays   int    `json:"direct_replays"`
	IndirectReplays int    `json:"indirect_replays"`
}

// limiter spaces calls to the same endpoint.
type limiter struct {
	mu   sync.Mutex
	next map[string]time.Time
}

func (l *limiter) wait(ctx context.Context, endpoint string, every time.Duration) error {
	l.mu.Lock()
	if l.next == nil {
		l.next = map[string]time.Time{}
	}
	at := time.Now()
	if n := l.next[endpoint]; n.After(at) {
		at = n
	}
	l.next[endpoint] = at.Add(every)
	l.mu.Unlock()
	if d := time.Until(at); d > 0 {
		return sleep(ctx, d)
	}
	return nil
}
