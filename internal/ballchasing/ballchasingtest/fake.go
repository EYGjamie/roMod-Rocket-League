// Package ballchasingtest provides a fake ballchasing API for tests and for
// trying the interface without an account.
package ballchasingtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
)

// Token is the only token the fake accepts.
const Token = "tok"

// Player is one player in a fake replay.
type Player struct {
	Name                                string
	PID                                 string // "steam:…"
	Score, Goals, Assists, Saves, Shots int
	MVP                                 bool
}

// Replay is a fake replay as the API would report it.
type Replay struct {
	ID           string
	Title        string
	PlaylistID   string
	PlaylistName string
	Map          string
	MatchGUID    string
	Date         time.Time
	Created      time.Time
	UploaderID   string // steam id; empty means the fake's own account
	UploaderName string
	Visibility   string
	Group        string
	Duration     int
	Blue, Orange []Player
	PendingPolls int // how often GET /replays/{id} still answers "pending"
	Failed       bool
	file         string
}

// Group is a fake ballchasing group.
type Group struct {
	ID, Name, Parent string
	Created          time.Time
}

// Server imitates the parts of the ballchasing API this tool uses.
type Server struct {
	*httptest.Server
	Me ballchasing.Me

	mu       sync.Mutex
	replays  map[string]*Replay
	groups   map[string]*Group
	seq      int
	patches  map[string][]string // replay ID -> group IDs it was patched to
	deleted  []string
	limitHit map[string]bool
	// Template builds the replay for an uploaded file. The default derives
	// the playlist from a file name like "ranked-doubles_x.replay".
	Template func(file string) Replay
	// Now is the upload time of new replays.
	Now func() time.Time
}

// New starts a fake with a fixed account (Jamie).
func New(t interface {
	Errorf(string, ...any)
	Cleanup(func())
}) *Server {
	f := &Server{
		Me:       ballchasing.Me{Name: "Jamie", SteamID: "76561198000000001"},
		replays:  map[string]*Replay{},
		groups:   map[string]*Group{},
		patches:  map[string][]string{},
		limitHit: map[string]bool{},
		Now:      time.Now,
	}
	f.Template = f.defaultTemplate
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"name": f.Me.Name, "steam_id": f.Me.SteamID, "type": "regular"})
	})
	mux.HandleFunc("POST /api/v2/upload", f.upload)
	mux.HandleFunc("GET /api/replays", f.list)
	mux.HandleFunc("GET /api/replays/{id}", f.get)
	mux.HandleFunc("PATCH /api/replays/{id}", f.patch)
	mux.HandleFunc("DELETE /api/replays/{id}", f.deleteReplay)
	mux.HandleFunc("GET /api/replays/{id}/file", f.download)
	mux.HandleFunc("GET /api/groups", f.listGroups)
	mux.HandleFunc("POST /api/groups", f.createGroup)
	mux.HandleFunc("DELETE /api/groups/{id}", f.deleteGroup)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// API returns a client for this server with the token it accepts.
func (f *Server) API() *ballchasing.Client {
	return &ballchasing.Client{BaseURL: f.URL, Token: Token, HTTPClient: f.Client(), Unpaced: true}
}

// AddReplay stores r and returns its ID (generated if empty).
func (f *Server) AddReplay(r Replay) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.add(r)
}

func (f *Server) add(r Replay) string {
	if r.ID == "" {
		f.seq++
		r.ID = fmt.Sprintf("r%d", f.seq)
	}
	if r.UploaderID == "" {
		r.UploaderID, r.UploaderName = f.Me.SteamID, f.Me.Name
	}
	if r.Visibility == "" {
		r.Visibility = "private"
	}
	if r.Created.IsZero() {
		r.Created = f.Now()
	}
	if r.Date.IsZero() {
		r.Date = r.Created.Add(-time.Minute)
	}
	if r.MatchGUID == "" {
		r.MatchGUID = "guid-" + r.ID
	}
	if r.Duration == 0 {
		r.Duration = 300
	}
	if r.Title == "" {
		r.Title = "Replay " + r.ID
	}
	if r.PlaylistName == "" {
		r.PlaylistName = PlaylistName(r.PlaylistID)
	}
	f.replays[r.ID] = &r
	return r.ID
}

// AddGroup stores g and returns its ID (generated if empty).
func (f *Server) AddGroup(g Group) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g.ID == "" {
		f.seq++
		g.ID = fmt.Sprintf("g%d", f.seq)
	}
	if g.Created.IsZero() {
		g.Created = f.Now()
	}
	f.groups[g.ID] = &g
	return g.ID
}

// Replay returns a copy of the stored replay.
func (f *Server) Replay(id string) (Replay, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.replays[id]
	if !ok {
		return Replay{}, false
	}
	return *r, true
}

// GroupByName returns the first group with that name.
func (f *Server) GroupByName(name string) (Group, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.groups {
		if g.Name == name {
			return *g, true
		}
	}
	return Group{}, false
}

// Patched returns the group IDs the replay was moved to, in order.
func (f *Server) Patched(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.patches[id])
}

// Deleted returns the IDs of deleted replays.
func (f *Server) Deleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.deleted)
}

var playlistNames = map[string]string{
	"ranked-duels": "Ranked Duels", "ranked-doubles": "Ranked Doubles", "ranked-standard": "Ranked Standard",
	"ranked-solo-standard": "Ranked Solo Standard", "unranked-duels": "Duels", "unranked-doubles": "Doubles",
	"unranked-standard": "Standard", "unranked-chaos": "Chaos", "private": "Private", "tournament": "Tournament",
	"ranked-hoops": "Ranked Hoops", "hoops": "Hoops", "ranked-rumble": "Ranked Rumble", "rumble": "Rumble",
	"ranked-dropshot": "Ranked Dropshot", "dropshot": "Dropshot", "ranked-snowday": "Ranked Snowday",
	"snowday": "Snowday", "heatseeker": "Heatseeker", "offline": "Offline",
}

// PlaylistName returns the display name ballchasing uses for a playlist.
func PlaylistName(id string) string {
	if n, ok := playlistNames[id]; ok {
		return n
	}
	return id
}

func (f *Server) defaultTemplate(file string) Replay {
	playlist := "ranked-doubles"
	if p, _, ok := strings.Cut(strings.TrimSuffix(file, ".replay"), "_"); ok && playlistNames[p] != "" {
		playlist = p
	}
	me := "steam:" + f.Me.SteamID
	return Replay{
		Title: strings.TrimSuffix(file, ".replay"), PlaylistID: playlist, Map: "DFH Stadium",
		Blue: []Player{
			{Name: f.Me.Name, PID: me, Score: 420, Goals: 2, Assists: 1, Saves: 2, Shots: 4, MVP: true},
			{Name: "Mate", PID: "steam:2", Score: 300, Goals: 1, Saves: 1, Shots: 2},
		},
		Orange: []Player{
			{Name: "Gegner A", PID: "epic:a", Score: 250, Goals: 1, Shots: 3},
			{Name: "Gegner B", PID: "epic:b", Score: 150, Saves: 3, Shots: 1},
		},
	}
}

func (f *Server) upload(w http.ResponseWriter, r *http.Request) {
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file"})
		return
	}
	io.ReadAll(file)
	name := hdr.Filename
	switch {
	case strings.HasPrefix(name, "bad"):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse replay"})
		return
	case strings.HasPrefix(name, "busy"):
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	case strings.HasPrefix(name, "boom"):
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rp := range f.replays {
		if rp.file == name {
			writeJSON(w, http.StatusConflict, map[string]string{"id": rp.ID, "error": "duplicate replay"})
			return
		}
	}
	rp := f.Template(name)
	rp.file = name
	if v := r.URL.Query().Get("visibility"); v != "" {
		rp.Visibility = v
	}
	id := f.add(rp)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "location": "https://ballchasing.com/replay/" + id})
}

func (f *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	var out []*Replay
	for _, rp := range f.replays {
		switch {
		case q.Get("uploader") == "me" && rp.UploaderID != f.Me.SteamID:
			continue
		case q.Get("player-id") != "" && !hasPlayer(rp, q.Get("player-id")):
			continue
		case q.Get("player-id") != "" && rp.Visibility == "private" && rp.UploaderID != f.Me.SteamID:
			continue
		}
		if after, err := time.Parse(time.RFC3339, q.Get("created-after")); err == nil && !rp.Created.After(after) {
			continue
		}
		out = append(out, rp)
	}
	sort.Slice(out, func(i, j int) bool {
		if q.Get("sort-dir") == "asc" {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].Created.After(out[j].Created)
	})
	count, _ := strconv.Atoi(q.Get("count"))
	if count <= 0 {
		count = 150
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	page := map[string]any{"count": len(out)}
	end := min(offset+count, len(out))
	if end < len(out) {
		nq := r.URL.Query()
		nq.Set("offset", strconv.Itoa(end))
		page["next"] = f.URL + "/api/replays?" + nq.Encode()
	}
	list := []any{}
	for _, rp := range out[min(offset, len(out)):end] {
		list = append(list, f.summary(rp))
	}
	f.mu.Unlock()
	page["list"] = list
	writeJSON(w, http.StatusOK, page)
}

func hasPlayer(rp *Replay, pid string) bool {
	for _, p := range append(slices.Clone(rp.Blue), rp.Orange...) {
		if p.PID == pid {
			return true
		}
	}
	return false
}

func (f *Server) groupRefs(rp *Replay) []any {
	if g, ok := f.groups[rp.Group]; ok {
		return []any{map[string]string{"id": g.ID, "name": g.Name, "link": f.URL + "/api/groups/" + g.ID}}
	}
	return []any{}
}

func splitPID(pid string) map[string]string {
	platform, id, _ := strings.Cut(pid, ":")
	return map[string]string{"platform": platform, "id": id}
}

func (f *Server) summary(rp *Replay) map[string]any {
	team := func(name string, ps []Player) map[string]any {
		goals := 0
		players := []any{}
		for _, p := range ps {
			goals += p.Goals
			players = append(players, map[string]any{"name": p.Name, "id": splitPID(p.PID), "score": p.Score, "mvp": p.MVP})
		}
		return map[string]any{"name": name, "goals": goals, "players": players}
	}
	return map[string]any{
		"id": rp.ID, "link": f.URL + "/api/replays/" + rp.ID, "rocket_league_id": "RL" + rp.ID,
		"replay_title": rp.Title, "map_code": strings.ToLower(strings.ReplaceAll(rp.Map, " ", "_")), "map_name": rp.Map,
		"playlist_id": rp.PlaylistID, "playlist_name": rp.PlaylistName, "duration": rp.Duration,
		"overtime": false, "date": rp.Date.Format(time.RFC3339), "created": rp.Created.Format(time.RFC3339),
		"uploader":   map[string]string{"steam_id": rp.UploaderID, "name": rp.UploaderName},
		"visibility": rp.Visibility, "groups": f.groupRefs(rp),
		"blue": team("", rp.Blue), "orange": team("", rp.Orange),
	}
}

func (f *Server) detail(rp *Replay) map[string]any {
	team := func(color string, ps []Player) map[string]any {
		var goals, shots, saves, assists, score int
		players := []any{}
		for _, p := range ps {
			goals, shots, saves, assists, score = goals+p.Goals, shots+p.Shots, saves+p.Saves, assists+p.Assists, score+p.Score
			core := map[string]any{"score": p.Score, "goals": p.Goals, "assists": p.Assists, "saves": p.Saves,
				"shots": p.Shots, "mvp": p.MVP, "shooting_percentage": pct(p.Goals, p.Shots)}
			players = append(players, map[string]any{
				"name": p.Name, "id": splitPID(p.PID), "mvp": p.MVP, "car_name": "Octane", "car_id": 23,
				"start_time": 0, "end_time": rp.Duration, "steering_sensitivity": 1.4,
				"camera": map[string]any{"fov": 110, "height": 100, "pitch": -4, "distance": 270, "stiffness": 0.5, "swivel_speed": 4, "transition_speed": 1.2},
				"stats": map[string]any{
					"core":        core,
					"boost":       map[string]any{"bpm": 380.5, "bcpm": 395.1, "avg_amount": 45.2, "amount_collected": 2100, "amount_stolen": 410, "time_zero_boost": 22.4, "percent_zero_boost": 7.5, "time_full_boost": 18.1, "percent_full_boost": 6.0},
					"movement":    map[string]any{"avg_speed": 1520, "total_distance": 420000, "time_supersonic_speed": 41.2, "percent_supersonic_speed": 13.7, "time_ground": 180.3, "percent_ground": 60.1, "time_low_air": 100.1, "time_high_air": 19.6, "count_powerslide": 54},
					"positioning": map[string]any{"avg_distance_to_ball": 2600, "time_defensive_third": 120.5, "time_neutral_third": 100.2, "time_offensive_third": 79.3, "percent_defensive_third": 40.2, "time_behind_ball": 210, "time_infront_ball": 90, "percent_behind_ball": 70},
					"demo":        map[string]any{"inflicted": 1, "taken": 0},
				},
			})
		}
		return map[string]any{"color": color, "players": players, "stats": map[string]any{
			"ball": map[string]any{"possession_time": 140.2, "time_in_side": 150.8},
			"core": map[string]any{"goals": goals, "shots": shots, "saves": saves, "assists": assists, "score": score, "shooting_percentage": pct(goals, shots)},
		}}
	}
	d := f.summary(rp)
	delete(d, "replay_title")
	d["title"] = rp.Title
	d["match_guid"] = rp.MatchGUID
	d["status"] = "ok"
	d["season"] = 21
	d["server"] = map[string]string{"name": "EU1", "region": "Europe"}
	d["blue"] = team("blue", rp.Blue)
	d["orange"] = team("orange", rp.Orange)
	return d
}

func pct(goals, shots int) float64 {
	if shots == 0 {
		return 0
	}
	return float64(goals) * 100 / float64(shots)
}

func (f *Server) get(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, ok := f.replays[r.PathValue("id")]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replay not found"})
		return
	}
	switch {
	case rp.PendingPolls > 0:
		rp.PendingPolls--
		writeJSON(w, http.StatusOK, map[string]string{"id": rp.ID, "status": "pending"})
	case rp.Failed:
		writeJSON(w, http.StatusOK, map[string]string{"id": rp.ID, "status": "failed", "error": "replay could not be parsed"})
	default:
		writeJSON(w, http.StatusOK, f.detail(rp))
	}
}

func (f *Server) patch(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	json.NewDecoder(r.Body).Decode(&in)
	id := r.PathValue("id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == "r-limit" && !f.limitHit[id] {
		f.limitHit[id] = true
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	rp, ok := f.replays[id]
	if !ok || rp.UploaderID != f.Me.SteamID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replay not found"})
		return
	}
	if g, ok := in["group"]; ok {
		if _, exists := f.groups[g]; g != "" && !exists {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group not found"})
			return
		}
		rp.Group = g
		f.patches[id] = append(f.patches[id], g)
	}
	if v, ok := in["visibility"]; ok {
		rp.Visibility = v
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *Server) deleteReplay(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	rp, ok := f.replays[id]
	if !ok || rp.UploaderID != f.Me.SteamID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replay not found"})
		return
	}
	delete(f.replays, id)
	f.deleted = append(f.deleted, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *Server) download(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	_, ok := f.replays[r.PathValue("id")]
	f.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replay not found"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	io.WriteString(w, "replay-bytes-"+r.PathValue("id"))
}

func (f *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("group")
	f.mu.Lock()
	var out []*Group
	for _, g := range f.groups {
		if g.Parent == parent {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	list := []any{}
	for _, g := range out {
		direct, indirect := f.counts(g.ID)
		list = append(list, map[string]any{"id": g.ID, "name": g.Name, "created": g.Created.Format(time.RFC3339),
			"direct_replays": direct, "indirect_replays": indirect, "link": f.URL + "/api/groups/" + g.ID})
	}
	f.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"list": list})
}

// counts returns the replays directly in a group and in it plus its
// subgroups.
func (f *Server) counts(id string) (direct, all int) {
	for _, rp := range f.replays {
		if rp.Group == id {
			direct++
		}
	}
	all = direct
	for _, g := range f.groups {
		if g.Parent == id {
			_, sub := f.counts(g.ID)
			all += sub
		}
	}
	return direct, all
}

func (f *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	json.NewDecoder(r.Body).Decode(&in)
	parent, hasParent := in["parent"]
	if in["name"] == "" || in["player_identification"] != "by-id" || in["team_identification"] == "" || hasParent && parent == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad group"})
		return
	}
	f.mu.Lock()
	if _, ok := f.groups[parent]; parent != "" && !ok {
		f.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent group not found"})
		return
	}
	f.mu.Unlock()
	id := f.AddGroup(Group{Name: in["name"], Parent: parent})
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "link": f.URL + "/api/groups/" + id})
}

func (f *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	if _, ok := f.groups[id]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}
	delete(f.groups, id)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
