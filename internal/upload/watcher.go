// Package upload watches the replay folders and uploads every new replay once.
package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
)

// State remembers which replay files were handled, so restarts neither
// upload twice nor miss replays saved while the tool was not running.
type State struct {
	path     string
	Uploaded map[string]string `json:"uploaded"` // file name -> ballchasing ID ("" = skipped)
}

// LoadState returns the saved state and whether a state file existed.
func LoadState(path string) (*State, bool, error) {
	s := &State{path: path, Uploaded: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if s.Uploaded == nil {
		s.Uploaded = map[string]string{}
	}
	return s, true, nil
}

func (s *State) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

type pendingFile struct {
	size    int64
	retryAt time.Time
	backoff time.Duration
}

// Watcher polls the replay folders and uploads every new .replay file once
// its size has stopped changing.
type Watcher struct {
	dirs    []string
	upload  func(ctx context.Context, path string) (id string, duplicate bool, err error)
	state   *State
	now     func() time.Time
	pending map[string]*pendingFile
}

func NewWatcher(dirs []string, state *State, upload func(context.Context, string) (string, bool, error)) *Watcher {
	return &Watcher{dirs: dirs, upload: upload, state: state, now: time.Now, pending: map[string]*pendingFile{}}
}

func (w *Watcher) replays(dir string) []fs.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // e.g. DemosEpic on a Steam install
	}
	var out []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".replay") {
			out = append(out, e)
		}
	}
	return out
}

// Seed marks all existing replays as handled so the first start does not
// upload the whole backlog.
func (w *Watcher) Seed() (int, error) {
	n := 0
	for _, dir := range w.dirs {
		for _, e := range w.replays(dir) {
			w.state.Uploaded[e.Name()] = ""
			n++
		}
	}
	return n, w.state.save()
}

func (w *Watcher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		w.scan(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (w *Watcher) scan(ctx context.Context) {
	for _, dir := range w.dirs {
		for _, e := range w.replays(dir) {
			name := e.Name()
			if _, ok := w.state.Uploaded[name]; ok {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(dir, name)
			p := w.pending[path]
			if p == nil || p.size != info.Size() || info.Size() == 0 {
				// new or still being written: wait for the size to settle
				if p == nil {
					p = &pendingFile{}
					w.pending[path] = p
				}
				p.size = info.Size()
				continue
			}
			if w.now().Before(p.retryAt) {
				continue
			}
			w.process(ctx, path, name, p)
		}
	}
}

func (w *Watcher) process(ctx context.Context, path, name string, p *pendingFile) {
	id, duplicate, err := w.upload(ctx, path)
	switch {
	case err == nil && duplicate:
		log.Printf("%s war schon auf ballchasing: https://ballchasing.com/replay/%s", name, id)
		w.done(path, name, id)
	case err == nil:
		log.Printf("%s hochgeladen: https://ballchasing.com/replay/%s", name, id)
		w.done(path, name, id)
	case errors.Is(err, ballchasing.ErrRejected):
		log.Printf("%s übersprungen: %v", name, err)
		w.done(path, name, "")
	case ctx.Err() != nil:
	default:
		p.backoff = min(max(2*p.backoff, 30*time.Second), 10*time.Minute)
		p.retryAt = w.now().Add(p.backoff)
		log.Printf("Upload von %s fehlgeschlagen, neuer Versuch in %s: %v", name, p.backoff, err)
	}
}

func (w *Watcher) done(path, name, id string) {
	delete(w.pending, path)
	w.state.Uploaded[name] = id
	if err := w.state.save(); err != nil {
		log.Printf("Status konnte nicht gespeichert werden: %v", err)
	}
}
