package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// States of a replay file.
const (
	Seeded     = "seeded"     // existed before the first start, not uploaded
	Queued     = "queued"     // waiting for its upload
	Uploading  = "uploading"  // upload running
	Processing = "processing" // uploaded, ballchasing still processing it
	Done       = "done"       // uploaded and processed
	Retry      = "retry"      // failed, another attempt at NextTry
	Failed     = "failed"     // failed three times, no more automatic attempts
	Rejected   = "rejected"   // ballchasing cannot process the file
)

// File is a replay file the tool knows about.
type File struct {
	Name      string // file name, unique across the replay folders
	Dir       string
	State     string
	ReplayID  string
	Attempts  int
	Error     string
	NextTry   time.Time
	Added     time.Time
	Updated   time.Time
	Deleted   bool // removed from disk after the upload
	Manual    bool // uploaded by hand, not from a replay folder
	Duplicate bool // was on ballchasing already when uploaded
}

const fileCols = "name, dir, state, replay_id, attempts, error, next_try, added, updated, deleted, manual, duplicate"

func scanFile(row interface{ Scan(...any) error }) (File, error) {
	var f File
	var next, added, updated int64
	var deleted, manual, duplicate int
	err := row.Scan(&f.Name, &f.Dir, &f.State, &f.ReplayID, &f.Attempts, &f.Error, &next, &added, &updated, &deleted, &manual, &duplicate)
	f.NextTry, f.Added, f.Updated = fromUnix(next), fromUnix(added), fromUnix(updated)
	f.Deleted, f.Manual, f.Duplicate = deleted != 0, manual != 0, duplicate != 0
	return f, err
}

// File returns the file with that name.
func (s *Store) File(name string) (File, bool, error) {
	f, err := scanFile(s.db.QueryRow("SELECT "+fileCols+" FROM files WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, false, nil
	}
	return f, err == nil, err
}

// PutFile inserts or replaces f. Added is set on the first insert.
func (s *Store) PutFile(f File) error {
	now := time.Now()
	if f.Added.IsZero() {
		f.Added = now
	}
	_, err := s.db.Exec(`INSERT INTO files(`+fileCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET dir=excluded.dir, state=excluded.state, replay_id=excluded.replay_id,
		attempts=excluded.attempts, error=excluded.error, next_try=excluded.next_try, updated=excluded.updated,
		deleted=excluded.deleted, manual=excluded.manual, duplicate=excluded.duplicate`,
		f.Name, f.Dir, f.State, f.ReplayID, f.Attempts, f.Error, unix(f.NextTry), unix(f.Added), unix(now),
		boolInt(f.Deleted), boolInt(f.Manual), boolInt(f.Duplicate))
	return err
}

// Files returns the files in the given states (all if none), oldest first.
func (s *Store) Files(states ...string) ([]File, error) {
	q := "SELECT " + fileCols + " FROM files"
	args := make([]any, len(states))
	if len(states) > 0 {
		q += " WHERE state IN (?" + strings.Repeat(",?", len(states)-1) + ")"
		for i, st := range states {
			args[i] = st
		}
	}
	rows, err := s.db.Query(q+" ORDER BY added, name", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FileNames returns the names of all known files.
func (s *Store) FileNames() (map[string]bool, error) {
	rows, err := s.db.Query("SELECT name FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names[n] = true
	}
	return names, rows.Err()
}

// CountFiles counts the files per state.
func (s *Store) CountFiles() (map[string]int, error) {
	rows, err := s.db.Query("SELECT state, COUNT(*) FROM files GROUP BY state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		counts[st] = n
	}
	return counts, rows.Err()
}

// ImportLegacy takes over the state file of earlier versions: file name ->
// ballchasing ID, where an empty ID means the file was only registered.
func (s *Store) ImportLegacy(uploaded map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	now := unix(time.Now())
	for name, id := range uploaded {
		state := Done
		if id == "" {
			state = Seeded
		}
		if _, err := tx.Exec("INSERT OR IGNORE INTO files(name, state, replay_id, added, updated) VALUES(?,?,?,?,?)",
			name, state, id, now, now); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
