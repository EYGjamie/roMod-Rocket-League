package app

import (
	"sort"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/rules"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

type playerView struct {
	Name    string `json:"name"`
	PID     string `json:"pid"`
	Score   int    `json:"score"`
	Goals   int    `json:"goals"`
	Assists int    `json:"assists"`
	Saves   int    `json:"saves"`
	Shots   int    `json:"shots"`
	Core    bool   `json:"core"` // goals to shots are known
	MVP     bool   `json:"mvp"`
	Me      bool   `json:"me"`
}

type teamView struct {
	Name    string       `json:"name"`
	Goals   int          `json:"goals"`
	Players []playerView `json:"players"`
}

// replayView is a replay as the interface shows it.
type replayView struct {
	Kind       string    `json:"kind"` // "replay"
	ID         string    `json:"id"`
	Own        bool      `json:"own"`
	Uploader   string    `json:"uploader"`
	Title      string    `json:"title"`
	Date       time.Time `json:"date"`
	Created    time.Time `json:"created"`
	PlaylistID string    `json:"playlist_id"`
	Playlist   string    `json:"playlist"`
	Map        string    `json:"map"`
	Duration   int       `json:"duration"`
	Overtime   bool      `json:"overtime"`
	Visibility string    `json:"visibility"`
	GroupID    string    `json:"group_id"`
	Group      string    `json:"group"` // path like "Liga › Spieltag 1"
	Blue       teamView  `json:"blue"`
	Orange     teamView  `json:"orange"`
	MyTeam     string    `json:"my_team"`
	Result     string    `json:"result"`
	File       string    `json:"file"`
	Local      bool      `json:"local"`    // the uploaded file is still on disk
	Borrowed   string    `json:"borrowed"` // "", "temp" or "kept"
	HasDetail  bool      `json:"has_detail"`
	Link       string    `json:"link"`
}

// fileView is a local file waiting for or failing its upload.
type fileView struct {
	Kind     string    `json:"kind"` // "file"
	Name     string    `json:"name"`
	State    string    `json:"state"`
	Error    string    `json:"error"`
	Attempts int       `json:"attempts"`
	NextTry  time.Time `json:"next_try"`
	Added    time.Time `json:"added"`
	ReplayID string    `json:"replay_id"`
	Manual   bool      `json:"manual"`
}

func toFileView(f store.File) fileView {
	return fileView{Kind: "file", Name: f.Name, State: f.State, Error: f.Error, Attempts: f.Attempts,
		NextTry: f.NextTry, Added: f.Added, ReplayID: f.ReplayID, Manual: f.Manual}
}

// viewCtx holds what converting many replays needs, loaded once.
type viewCtx struct {
	me       map[string]bool
	paths    map[string]string // group ID -> path
	borrowed map[string]bool
	local    map[string]bool // file names still on disk
}

func (a *App) newViewCtx() viewCtx {
	groups, _ := a.store.Groups()
	paths := map[string]string{}
	for _, g := range groups {
		paths[g.ID] = a.groupPath(groups, g.ID)
	}
	local := map[string]bool{}
	files, _ := a.store.Files(store.Done)
	for _, f := range files {
		if !f.Deleted {
			local[f.Name] = true
		}
	}
	return viewCtx{me: a.myIDs(), paths: paths, borrowed: a.borrowed(), local: local}
}

func (vc viewCtx) replay(r store.Replay) replayView {
	v := replayView{Kind: "replay", ID: r.ID, Own: r.Own, Uploader: r.UploaderName, Title: r.Title, Date: r.Date,
		Created: r.Created, PlaylistID: rules.Canonical(r.PlaylistID), Playlist: rules.PlaylistName(r.PlaylistID), Map: r.MapName,
		Duration: r.Duration, Overtime: r.Overtime, Visibility: r.Visibility, GroupID: r.GroupID, Group: vc.paths[r.GroupID],
		MyTeam: r.MyTeam(vc.me), Result: r.Result(vc.me), File: r.File, Local: r.File != "" && vc.local[r.File],
		HasDetail: r.HasDetail, Link: "https://ballchasing.com/replay/" + r.ID,
		Blue: teamView{Name: r.BlueName, Goals: r.BlueGoals, Players: []playerView{}}, Orange: teamView{Name: r.OrangeName, Goals: r.OrangeGoals, Players: []playerView{}}}
	if v.Group == "" {
		v.Group = r.GroupName
	}
	if keep, ok := vc.borrowed[r.ID]; ok {
		v.Borrowed = "temp"
		if keep {
			v.Borrowed = "kept"
		}
	}
	for _, p := range r.Players {
		pv := playerView{Name: p.Name, PID: p.PID, Score: p.Score, Goals: p.Goals, Assists: p.Assists, Saves: p.Saves,
			Shots: p.Shots, Core: p.Core, MVP: p.MVP, Me: vc.me[p.PID]}
		if p.Team == "orange" {
			v.Orange.Players = append(v.Orange.Players, pv)
		} else {
			v.Blue.Players = append(v.Blue.Players, pv)
		}
	}
	return v
}

// listQuery filters the history.
type listQuery struct {
	Q        string
	Status   string // all, uploaded, failed
	Playlist string
	Group    string // group ID or "unsorted"
	Origin   string // all, own, mates
	Days     int
	Offset   int
	Limit    int
}

type listResult struct {
	Total int   `json:"total"`
	Items []any `json:"items"`
}

func (vc viewCtx) matches(r store.Replay, q listQuery, since time.Time) bool {
	switch {
	case q.Origin == "own" && !r.Own, q.Origin == "mates" && r.Own:
		return false
	case q.Playlist != "" && rules.Canonical(r.PlaylistID) != rules.Canonical(q.Playlist):
		return false
	case q.Group == "unsorted" && (!r.Own || r.GroupID != ""):
		return false
	case q.Group != "" && q.Group != "unsorted" && r.GroupID != q.Group:
		return false
	case !since.IsZero() && r.Date.Before(since):
		return false
	}
	if q.Q == "" {
		return true
	}
	hay := []string{r.MapName, r.Title, r.File, r.UploaderName, r.BlueName, r.OrangeName, rules.PlaylistName(r.PlaylistID)}
	for _, p := range r.Players {
		hay = append(hay, p.Name)
	}
	return strings.Contains(strings.ToLower(strings.Join(hay, "\x00")), strings.ToLower(q.Q))
}

// list returns the history rows: replays and, unless only uploaded ones
// are wanted, the files whose upload failed.
func (a *App) list(q listQuery) listResult {
	vc := a.newViewCtx()
	var since time.Time
	if q.Days > 0 {
		since = a.now().AddDate(0, 0, -q.Days)
	}
	type row struct {
		at   time.Time
		item any
	}
	var rows []row
	if q.Status != "failed" {
		for _, r := range a.visible() {
			if vc.matches(r, q, since) {
				rows = append(rows, row{r.Date, vc.replay(r)})
			}
		}
	}
	if q.Status != "uploaded" && q.Group == "" && q.Playlist == "" && q.Origin != "mates" {
		files, _ := a.store.Files(store.Failed, store.Rejected, store.Retry)
		for _, f := range files {
			if (since.IsZero() || !f.Added.Before(since)) && (q.Q == "" || strings.Contains(strings.ToLower(f.Name), strings.ToLower(q.Q))) {
				rows = append(rows, row{f.Added, toFileView(f)})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at.After(rows[j].at) })
	res := listResult{Total: len(rows), Items: []any{}}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	for i := q.Offset; i < len(rows) && i < q.Offset+q.Limit; i++ {
		res.Items = append(res.Items, rows[i].item)
	}
	return res
}
