// Package store keeps the tool's state in a local SQLite database: the
// replay files and their upload state, a copy of the replays on ballchasing,
// the group tree, sorting rules and decisions, and the lobbies of the
// developer mode's rank lookup.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath() +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer is plenty and avoids SQLITE_BUSY
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

var migrations = []string{`
CREATE TABLE files (
	name      TEXT PRIMARY KEY,
	dir       TEXT NOT NULL DEFAULT '',
	state     TEXT NOT NULL,
	replay_id TEXT NOT NULL DEFAULT '',
	attempts  INTEGER NOT NULL DEFAULT 0,
	error     TEXT NOT NULL DEFAULT '',
	next_try  INTEGER NOT NULL DEFAULT 0,
	added     INTEGER NOT NULL DEFAULT 0,
	updated   INTEGER NOT NULL DEFAULT 0,
	deleted   INTEGER NOT NULL DEFAULT 0,
	manual    INTEGER NOT NULL DEFAULT 0,
	duplicate INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX files_state ON files(state);
CREATE TABLE replays (
	id            TEXT PRIMARY KEY,
	own           INTEGER NOT NULL DEFAULT 0,
	uploader_id   TEXT NOT NULL DEFAULT '',
	uploader_name TEXT NOT NULL DEFAULT '',
	match_guid    TEXT NOT NULL DEFAULT '',
	rl_id         TEXT NOT NULL DEFAULT '',
	title         TEXT NOT NULL DEFAULT '',
	date          INTEGER NOT NULL DEFAULT 0,
	created       INTEGER NOT NULL DEFAULT 0,
	playlist_id   TEXT NOT NULL DEFAULT '',
	playlist_name TEXT NOT NULL DEFAULT '',
	map_code      TEXT NOT NULL DEFAULT '',
	map_name      TEXT NOT NULL DEFAULT '',
	duration      INTEGER NOT NULL DEFAULT 0,
	overtime      INTEGER NOT NULL DEFAULT 0,
	visibility    TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT '',
	group_id      TEXT NOT NULL DEFAULT '',
	group_name    TEXT NOT NULL DEFAULT '',
	blue_name     TEXT NOT NULL DEFAULT '',
	blue_goals    INTEGER NOT NULL DEFAULT 0,
	orange_name   TEXT NOT NULL DEFAULT '',
	orange_goals  INTEGER NOT NULL DEFAULT 0,
	players       TEXT NOT NULL DEFAULT '[]',
	detail        TEXT,
	file          TEXT NOT NULL DEFAULT '',
	dup_of        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX replays_date ON replays(date);
CREATE TABLE groups (
	id       TEXT PRIMARY KEY,
	name     TEXT NOT NULL,
	parent   TEXT NOT NULL DEFAULT '',
	created  INTEGER NOT NULL DEFAULT 0,
	direct   INTEGER NOT NULL DEFAULT 0,
	indirect INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE rules (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	pos         INTEGER NOT NULL DEFAULT 0,
	enabled     INTEGER NOT NULL DEFAULT 1,
	playlists   TEXT NOT NULL DEFAULT '[]',
	players     TEXT NOT NULL DEFAULT '[]',
	all_players INTEGER NOT NULL DEFAULT 0,
	target_id   TEXT NOT NULL DEFAULT '',
	target_name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE decisions (
	replay_id   TEXT PRIMARY KEY,
	playlist_id TEXT NOT NULL DEFAULT '',
	mates       TEXT NOT NULL DEFAULT '[]',
	group_id    TEXT NOT NULL DEFAULT '',
	manual      INTEGER NOT NULL DEFAULT 0,
	at          INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE identities (
	pid       TEXT PRIMARY KEY,
	name      TEXT NOT NULL DEFAULT '',
	confirmed INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE borrows (
	name      TEXT PRIMARY KEY,
	replay_id TEXT NOT NULL,
	keep      INTEGER NOT NULL DEFAULT 0,
	added     INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE ignored (key TEXT PRIMARY KEY);
`, `
CREATE INDEX replays_score ON replays(map_code, blue_goals, orange_goals);
`, `
ALTER TABLE rules ADD COLUMN subfolders TEXT NOT NULL DEFAULT '';
`, `
CREATE TABLE lobbies (
	guid    TEXT PRIMARY KEY,
	started INTEGER NOT NULL DEFAULT 0,
	size    INTEGER NOT NULL DEFAULT 0,
	smurf   INTEGER NOT NULL DEFAULT 0,
	winner  INTEGER NOT NULL DEFAULT -1,
	players TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX lobbies_started ON lobbies(started);
`}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("Migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// unix stores times as seconds; the zero time becomes 0.
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Meta returns a stored setting such as a sync cursor, "" if unset.
func (s *Store) Meta(key string) string {
	var v string
	s.db.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&v)
	return v
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec("INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

// Ignored reports whether a suggestion was dismissed.
func (s *Store) Ignored(key string) bool {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM ignored WHERE key = ?", key).Scan(&n)
	return n > 0
}

func (s *Store) Ignore(key string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO ignored(key) VALUES(?)", key)
	return err
}
