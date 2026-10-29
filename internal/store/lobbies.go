package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Lobby is an online match as the Stats API listed it, with the ranks the
// lookup found at its start.
type Lobby struct {
	GUID    string        `json:"guid"`
	Started time.Time     `json:"started"`
	Size    int           `json:"size"`   // players per team
	Smurf   bool          `json:"smurf"`  // marked by the user: a smurf in the lobby
	Winner  int           `json:"winner"` // 0 blue, 1 orange, -1 not known
	Players []LobbyPlayer `json:"players"`
}

// LobbyPlayer is one player of a lobby.
type LobbyPlayer struct {
	ID     string   `json:"id"` // PrimaryId of the Stats API, e.g. Steam|7656…|0
	Name   string   `json:"name"`
	Team   int      `json:"team"`
	Me     bool     `json:"me"`
	Left   bool     `json:"left,omitempty"`   // no longer in the match
	Looked bool     `json:"looked,omitempty"` // the lookup ran for this player
	Ranks  [3]*Rank `json:"ranks"`            // 1s, 2s, 3s; nil if not known
	Wins   int      `json:"wins"`             // all-time wins, -1 if not known
}

// Rank is a player's rank in a ranked playlist.
type Rank struct {
	Tier   int     `json:"tier"` // 0 unranked, 1 Bronze I … 22 Supersonic Legend
	Div    int     `json:"div"`  // division 0–3
	MMR    float64 `json:"mmr"`
	Played int     `json:"played"` // matches this season
}

// PutLobby adds or replaces a lobby.
func (s *Store) PutLobby(l Lobby) error {
	_, err := s.db.Exec(`INSERT INTO lobbies(guid, started, size, smurf, winner, players) VALUES(?,?,?,?,?,?)
		ON CONFLICT(guid) DO UPDATE SET started=excluded.started, size=excluded.size, smurf=excluded.smurf,
		winner=excluded.winner, players=excluded.players`,
		l.GUID, unix(l.Started), l.Size, boolInt(l.Smurf), l.Winner, toJSON(l.Players))
	return err
}

// Lobby returns one lobby; ok is false if there is none with guid.
func (s *Store) Lobby(guid string) (Lobby, bool, error) {
	list, err := s.queryLobbies("SELECT guid, started, size, smurf, winner, players FROM lobbies WHERE guid = ?", guid)
	if err != nil || len(list) == 0 {
		return Lobby{}, false, err
	}
	return list[0], true, nil
}

// Lobbies returns the lobbies started since since, oldest first.
func (s *Store) Lobbies(since time.Time) ([]Lobby, error) {
	return s.queryLobbies("SELECT guid, started, size, smurf, winner, players FROM lobbies WHERE started >= ? ORDER BY started", unix(since))
}

// SetSmurf marks or unmarks a lobby.
func (s *Store) SetSmurf(guid string, smurf bool) error {
	res, err := s.db.Exec("UPDATE lobbies SET smurf = ? WHERE guid = ?", boolInt(smurf), guid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) queryLobbies(query string, args ...any) ([]Lobby, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lobby
	for rows.Next() {
		var l Lobby
		var started int64
		var smurf int
		var players string
		if err := rows.Scan(&l.GUID, &started, &l.Size, &smurf, &l.Winner, &players); err != nil {
			return nil, err
		}
		l.Started, l.Smurf = fromUnix(started), smurf != 0
		if err := json.Unmarshal([]byte(players), &l.Players); err != nil {
			return nil, errors.Join(errors.New("Lobby "+l.GUID), err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
