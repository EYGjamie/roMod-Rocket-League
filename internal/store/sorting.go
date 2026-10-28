package store

import (
	"encoding/json"
	"sort"
	"time"
)

// Group is a ballchasing group, shown as a folder.
type Group struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Parent   string    `json:"parent"`
	Created  time.Time `json:"created"`
	Direct   int       `json:"direct"`   // replays directly in the group
	Indirect int       `json:"indirect"` // replays in the group and its subgroups
}

// ReplaceGroups stores the complete group tree.
func (s *Store) ReplaceGroups(groups []Group) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM groups"); err != nil {
		tx.Rollback()
		return err
	}
	for _, g := range groups {
		if _, err := tx.Exec("INSERT INTO groups(id, name, parent, created, direct, indirect) VALUES(?,?,?,?,?,?)",
			g.ID, g.Name, g.Parent, unix(g.Created), g.Direct, g.Indirect); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// PutGroup adds or updates one group.
func (s *Store) PutGroup(g Group) error {
	_, err := s.db.Exec(`INSERT INTO groups(id, name, parent, created, direct, indirect) VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, parent=excluded.parent, direct=excluded.direct, indirect=excluded.indirect`,
		g.ID, g.Name, g.Parent, unix(g.Created), g.Direct, g.Indirect)
	return err
}

// DeleteGroup removes a group from the tree.
func (s *Store) DeleteGroup(id string) error {
	_, err := s.db.Exec("DELETE FROM groups WHERE id = ?", id)
	return err
}

// Groups returns all groups ordered by name.
func (s *Store) Groups() ([]Group, error) {
	rows, err := s.db.Query("SELECT id, name, parent, created, direct, indirect FROM groups ORDER BY name COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var created int64
		if err := rows.Scan(&g.ID, &g.Name, &g.Parent, &created, &g.Direct, &g.Indirect); err != nil {
			return nil, err
		}
		g.Created = fromUnix(created)
		out = append(out, g)
	}
	return out, rows.Err()
}

// Rule sorts uploaded replays into a group.
type Rule struct {
	ID         int64    `json:"id"`
	Pos        int      `json:"pos"`
	Enabled    bool     `json:"enabled"`
	Playlists  []string `json:"playlists"`   // any of these playlists, all if empty
	Players    []string `json:"players"`     // teammates by ID, ignored if empty
	AllPlayers bool     `json:"all_players"` // all teammates must play instead of any
	TargetID   string   `json:"target_id"`   // group ID, empty until created
	TargetName string   `json:"target_name"` // name for creating the group
	Subfolders string   `json:"subfolders"`  // DailySubfolders or empty
}

// DailySubfolders sorts into one subfolder of the target per day the game
// was played, named like 2026-10-28.
const DailySubfolders = "day"

// Rules returns the rules in their order.
func (s *Store) Rules() ([]Rule, error) {
	rows, err := s.db.Query("SELECT id, pos, enabled, playlists, players, all_players, target_id, target_name, subfolders FROM rules ORDER BY pos, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		var enabled, all int
		var playlists, players string
		if err := rows.Scan(&r.ID, &r.Pos, &enabled, &playlists, &players, &all, &r.TargetID, &r.TargetName, &r.Subfolders); err != nil {
			return nil, err
		}
		r.Enabled, r.AllPlayers = enabled != 0, all != 0
		json.Unmarshal([]byte(playlists), &r.Playlists)
		json.Unmarshal([]byte(players), &r.Players)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutRule inserts r (ID 0, appended at the end) or updates it.
func (s *Store) PutRule(r *Rule) error {
	if r.Playlists == nil {
		r.Playlists = []string{}
	}
	if r.Players == nil {
		r.Players = []string{}
	}
	if r.ID == 0 {
		if r.Pos == 0 {
			s.db.QueryRow("SELECT COALESCE(MAX(pos), 0) + 1 FROM rules").Scan(&r.Pos)
		}
		res, err := s.db.Exec("INSERT INTO rules(pos, enabled, playlists, players, all_players, target_id, target_name, subfolders) VALUES(?,?,?,?,?,?,?,?)",
			r.Pos, boolInt(r.Enabled), toJSON(r.Playlists), toJSON(r.Players), boolInt(r.AllPlayers), r.TargetID, r.TargetName, r.Subfolders)
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.Exec("UPDATE rules SET pos=?, enabled=?, playlists=?, players=?, all_players=?, target_id=?, target_name=?, subfolders=? WHERE id=?",
		r.Pos, boolInt(r.Enabled), toJSON(r.Playlists), toJSON(r.Players), boolInt(r.AllPlayers), r.TargetID, r.TargetName, r.Subfolders, r.ID)
	return err
}

func (s *Store) DeleteRule(id int64) error {
	_, err := s.db.Exec("DELETE FROM rules WHERE id = ?", id)
	return err
}

// OrderRules puts the rules in the order of ids.
func (s *Store) OrderRules(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE rules SET pos = ? WHERE id = ?", i+1, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Decision records where a replay was sorted, by hand or by a rule.
type Decision struct {
	ReplayID   string
	PlaylistID string
	Mates      []string
	GroupID    string
	Manual     bool
	At         time.Time
}

func (s *Store) PutDecision(d Decision) error {
	if d.At.IsZero() {
		d.At = time.Now()
	}
	if d.Mates == nil {
		d.Mates = []string{}
	}
	sort.Strings(d.Mates)
	_, err := s.db.Exec(`INSERT INTO decisions(replay_id, playlist_id, mates, group_id, manual, at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(replay_id) DO UPDATE SET playlist_id=excluded.playlist_id, mates=excluded.mates, group_id=excluded.group_id,
		manual=excluded.manual, at=excluded.at`,
		d.ReplayID, d.PlaylistID, toJSON(d.Mates), d.GroupID, boolInt(d.Manual), unix(d.At))
	return err
}

// Decisions returns all decisions, newest first.
func (s *Store) Decisions() ([]Decision, error) {
	rows, err := s.db.Query("SELECT replay_id, playlist_id, mates, group_id, manual, at FROM decisions ORDER BY at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Decision
	for rows.Next() {
		var d Decision
		var mates string
		var manual int
		var at int64
		if err := rows.Scan(&d.ReplayID, &d.PlaylistID, &mates, &d.GroupID, &manual, &at); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(mates), &d.Mates)
		d.Manual, d.At = manual != 0, fromUnix(at)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Identity is one of the user's player accounts.
type Identity struct {
	PID       string `json:"pid"`
	Name      string `json:"name"`
	Confirmed bool   `json:"confirmed"`
}

func (s *Store) Identities() ([]Identity, error) {
	rows, err := s.db.Query("SELECT pid, name, confirmed FROM identities ORDER BY pid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var id Identity
		var confirmed int
		if err := rows.Scan(&id.PID, &id.Name, &confirmed); err != nil {
			return nil, err
		}
		id.Confirmed = confirmed != 0
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetIdentities replaces the user's accounts.
func (s *Store) SetIdentities(ids []Identity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM identities"); err != nil {
		tx.Rollback()
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec("INSERT INTO identities(pid, name, confirmed) VALUES(?,?,?)", id.PID, id.Name, boolInt(id.Confirmed)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Borrow is a replay file downloaded for watching in the game.
type Borrow struct {
	Name     string
	ReplayID string
	Keep     bool
	Added    time.Time
}

func (s *Store) PutBorrow(b Borrow) error {
	if b.Added.IsZero() {
		b.Added = time.Now()
	}
	_, err := s.db.Exec(`INSERT INTO borrows(name, replay_id, keep, added) VALUES(?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET keep=excluded.keep`, b.Name, b.ReplayID, boolInt(b.Keep), unix(b.Added))
	return err
}

func (s *Store) Borrows() ([]Borrow, error) {
	rows, err := s.db.Query("SELECT name, replay_id, keep, added FROM borrows ORDER BY added")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Borrow
	for rows.Next() {
		var b Borrow
		var keep int
		var added int64
		if err := rows.Scan(&b.Name, &b.ReplayID, &keep, &added); err != nil {
			return nil, err
		}
		b.Keep, b.Added = keep != 0, fromUnix(added)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBorrow(name string) error {
	_, err := s.db.Exec("DELETE FROM borrows WHERE name = ?", name)
	return err
}
