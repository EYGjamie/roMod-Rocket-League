package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/EYGjamie/romod/internal/ballchasing"
)

// Player is one player of a stored replay.
type Player struct {
	Team    string `json:"team"` // "blue" or "orange"
	Name    string `json:"name"`
	PID     string `json:"pid"`
	Score   int    `json:"score"`
	MVP     bool   `json:"mvp,omitempty"`
	Core    bool   `json:"core,omitempty"` // goals to shots are known
	Goals   int    `json:"goals,omitempty"`
	Assists int    `json:"assists,omitempty"`
	Saves   int    `json:"saves,omitempty"`
	Shots   int    `json:"shots,omitempty"`
}

// Replay is the local copy of a replay on ballchasing.
type Replay struct {
	ID             string
	Own            bool // uploaded by this account
	UploaderID     string
	UploaderName   string
	MatchGUID      string
	RocketLeagueID string
	Title          string
	Date           time.Time // when the game was played
	Created        time.Time // when it was uploaded
	PlaylistID     string
	PlaylistName   string
	MapCode        string
	MapName        string
	Duration       int
	Overtime       bool
	Visibility     string
	Status         string
	GroupID        string
	GroupName      string
	BlueName       string
	BlueGoals      int
	OrangeName     string
	OrangeGoals    int
	Players        []Player
	HasDetail      bool
	File           string // local file the tool uploaded it from
	DupOf          string // another upload of the same game that is shown instead
}

// FromAPI converts a replay from either endpoint. ownSteamID is the steam
// ID of the account the token belongs to.
func FromAPI(r ballchasing.Replay, ownSteamID string) Replay {
	out := Replay{
		ID: r.ID, Own: r.Uploader.SteamID == ownSteamID, UploaderID: r.Uploader.SteamID, UploaderName: r.Uploader.Name,
		MatchGUID: r.MatchGUID, RocketLeagueID: r.RocketLeagueID, Title: r.Name(),
		Date: parseTime(r.Date), Created: parseTime(r.Created),
		PlaylistID: r.PlaylistID, PlaylistName: r.PlaylistName, MapCode: r.MapCode, MapName: r.MapName,
		Duration: int(r.Duration), Overtime: r.Overtime, Visibility: r.Visibility, Status: r.Status,
		BlueName: r.Blue.Name, BlueGoals: r.Blue.TotalGoals(), OrangeName: r.Orange.Name, OrangeGoals: r.Orange.TotalGoals(),
	}
	if len(r.Groups) > 0 {
		out.GroupID, out.GroupName = r.Groups[0].ID, r.Groups[0].Name
	}
	for team, t := range map[string]ballchasing.Team{"blue": r.Blue, "orange": r.Orange} {
		for _, p := range t.Players {
			sp := Player{Team: team, Name: p.Name, PID: p.PID(), Score: p.Score, MVP: p.MVP}
			if p.Stats != nil {
				c := p.Stats.Core
				sp.Core, sp.Score, sp.Goals, sp.Assists, sp.Saves, sp.Shots = true, c.Score, c.Goals, c.Assists, c.Saves, c.Shots
				sp.MVP = sp.MVP || c.MVP
			}
			out.Players = append(out.Players, sp)
		}
	}
	sortPlayers(out.Players)
	return out
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// sortPlayers orders blue before orange, then by score.
func sortPlayers(ps []Player) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && less(ps[j], ps[j-1]); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

func less(a, b Player) bool {
	if a.Team != b.Team {
		return a.Team == "blue"
	}
	return a.Score > b.Score
}

// Result says how the game went for a player with one of the given IDs:
// "win", "loss" or "" if none of them played.
func (r Replay) Result(myIDs map[string]bool) string {
	team := r.MyTeam(myIDs)
	switch {
	case team == "":
		return ""
	case r.BlueGoals == r.OrangeGoals:
		return ""
	case (team == "blue") == (r.BlueGoals > r.OrangeGoals):
		return "win"
	default:
		return "loss"
	}
}

// MyTeam returns "blue", "orange" or "" if none of the IDs played.
func (r Replay) MyTeam(myIDs map[string]bool) string {
	for _, p := range r.Players {
		if myIDs[p.PID] {
			return p.Team
		}
	}
	return ""
}

// Mates returns the IDs of the other players in my team.
func (r Replay) Mates(myIDs map[string]bool) []string {
	team := r.MyTeam(myIDs)
	var mates []string
	for _, p := range r.Players {
		if team != "" && p.Team == team && !myIDs[p.PID] && p.PID != "" {
			mates = append(mates, p.PID)
		}
	}
	return mates
}

const replayCols = `id, own, uploader_id, uploader_name, match_guid, rl_id, title, date, created, playlist_id, playlist_name,
	map_code, map_name, duration, overtime, visibility, status, group_id, group_name, blue_name, blue_goals,
	orange_name, orange_goals, players, detail IS NOT NULL, file, dup_of`

func scanReplay(row interface{ Scan(...any) error }) (Replay, error) {
	var r Replay
	var own, overtime, hasDetail int
	var date, created int64
	var players string
	err := row.Scan(&r.ID, &own, &r.UploaderID, &r.UploaderName, &r.MatchGUID, &r.RocketLeagueID, &r.Title, &date, &created,
		&r.PlaylistID, &r.PlaylistName, &r.MapCode, &r.MapName, &r.Duration, &overtime, &r.Visibility, &r.Status,
		&r.GroupID, &r.GroupName, &r.BlueName, &r.BlueGoals, &r.OrangeName, &r.OrangeGoals, &players, &hasDetail, &r.File, &r.DupOf)
	r.Own, r.Overtime, r.HasDetail = own != 0, overtime != 0, hasDetail != 0
	r.Date, r.Created = fromUnix(date), fromUnix(created)
	json.Unmarshal([]byte(players), &r.Players)
	return r, err
}

// PutReplay inserts or updates the summary of r. The stored statistics, the
// local file and the duplicate mark are kept unless r sets them; player
// statistics are kept when r only carries the list summary.
func (s *Store) PutReplay(r Replay) error {
	if old, ok, err := s.Replay(r.ID); err != nil {
		return err
	} else if ok {
		if r.File == "" {
			r.File = old.File
		}
		if r.DupOf == "" {
			r.DupOf = old.DupOf
		}
		if r.MatchGUID == "" {
			r.MatchGUID = old.MatchGUID
		}
		if r.Status == "" {
			r.Status = old.Status
		}
		if !hasCore(r.Players) && hasCore(old.Players) {
			r.Players = old.Players
		}
	}
	if r.Players == nil {
		r.Players = []Player{}
	}
	_, err := s.db.Exec(`INSERT INTO replays(id, own, uploader_id, uploader_name, match_guid, rl_id, title, date, created,
		playlist_id, playlist_name, map_code, map_name, duration, overtime, visibility, status, group_id, group_name,
		blue_name, blue_goals, orange_name, orange_goals, players, file, dup_of)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET own=excluded.own, uploader_id=excluded.uploader_id, uploader_name=excluded.uploader_name,
		match_guid=excluded.match_guid, rl_id=excluded.rl_id, title=excluded.title, date=excluded.date, created=excluded.created,
		playlist_id=excluded.playlist_id, playlist_name=excluded.playlist_name, map_code=excluded.map_code, map_name=excluded.map_name,
		duration=excluded.duration, overtime=excluded.overtime, visibility=excluded.visibility, status=excluded.status,
		group_id=excluded.group_id, group_name=excluded.group_name, blue_name=excluded.blue_name, blue_goals=excluded.blue_goals,
		orange_name=excluded.orange_name, orange_goals=excluded.orange_goals, players=excluded.players, file=excluded.file,
		dup_of=excluded.dup_of`,
		r.ID, boolInt(r.Own), r.UploaderID, r.UploaderName, r.MatchGUID, r.RocketLeagueID, r.Title, unix(r.Date), unix(r.Created),
		r.PlaylistID, r.PlaylistName, r.MapCode, r.MapName, r.Duration, boolInt(r.Overtime), r.Visibility, r.Status,
		r.GroupID, r.GroupName, r.BlueName, r.BlueGoals, r.OrangeName, r.OrangeGoals, toJSON(r.Players), r.File, r.DupOf)
	return err
}

func hasCore(ps []Player) bool {
	for _, p := range ps {
		if p.Core {
			return true
		}
	}
	return false
}

// PutDetail stores a fully processed replay together with the raw reply of
// GET /replays/{id}.
func (s *Store) PutDetail(r Replay, raw []byte) error {
	if err := s.PutReplay(r); err != nil {
		return err
	}
	_, err := s.db.Exec("UPDATE replays SET detail = ? WHERE id = ?", string(raw), r.ID)
	return err
}

// Replay returns one replay without the raw statistics.
func (s *Store) Replay(id string) (Replay, bool, error) {
	r, err := scanReplay(s.db.QueryRow("SELECT "+replayCols+" FROM replays WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Replay{}, false, nil
	}
	return r, err == nil, err
}

// Replays returns all replays, newest game first.
func (s *Store) Replays() ([]Replay, error) {
	return s.queryReplays("SELECT " + replayCols + " FROM replays ORDER BY date DESC, id")
}

// ReplaysWithScore returns the replays on one map with one final score, in
// the same order as Replays.
func (s *Store) ReplaysWithScore(mapCode string, blue, orange int) ([]Replay, error) {
	return s.queryReplays("SELECT "+replayCols+" FROM replays WHERE map_code = ? AND blue_goals = ? AND orange_goals = ? ORDER BY date DESC, id",
		mapCode, blue, orange)
}

func (s *Store) queryReplays(query string, args ...any) ([]Replay, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Replay
	for rows.Next() {
		r, err := scanReplay(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Detail returns the raw statistics of a replay, nil if not fetched yet.
func (s *Store) Detail(id string) ([]byte, error) {
	var d sql.NullString
	err := s.db.QueryRow("SELECT detail FROM replays WHERE id = ?", id).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) || !d.Valid {
		return nil, nil
	}
	return []byte(d.String), err
}

// WithoutDetail returns up to n IDs of replays whose statistics are missing,
// own uploads and newer games first.
func (s *Store) WithoutDetail(n int) ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM replays WHERE detail IS NULL AND dup_of = '' ORDER BY own DESC, date DESC LIMIT ?", n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetReplayGroup records the group a replay was moved to.
func (s *Store) SetReplayGroup(id, groupID, groupName string) error {
	_, err := s.db.Exec("UPDATE replays SET group_id = ?, group_name = ? WHERE id = ?", groupID, groupName, id)
	return err
}

// SetVisibility records a changed visibility.
func (s *Store) SetVisibility(id, visibility string) error {
	_, err := s.db.Exec("UPDATE replays SET visibility = ? WHERE id = ?", visibility, id)
	return err
}

// SetDupOf hides a replay behind another upload of the same game, or shows
// it again with an empty of.
func (s *Store) SetDupOf(id, of string) error {
	_, err := s.db.Exec("UPDATE replays SET dup_of = ? WHERE id = ?", of, id)
	return err
}

// DeleteReplay removes a replay that was deleted on ballchasing.
func (s *Store) DeleteReplay(id string) error {
	_, err := s.db.Exec("DELETE FROM replays WHERE id = ?", id)
	return err
}

// RenameGroupInReplays updates the group name shown with replays.
func (s *Store) RenameGroupInReplays(groupID, name string) error {
	_, err := s.db.Exec("UPDATE replays SET group_name = ? WHERE group_id = ?", name, groupID)
	return err
}
