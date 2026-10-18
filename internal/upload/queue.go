// Package upload watches the replay folders and uploads every new replay
// once, with retries, until ballchasing has processed it.
package upload

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// API is the part of the ballchasing client the queue needs.
type API interface {
	Upload(ctx context.Context, path, visibility string) (id string, duplicate bool, err error)
	GetReplay(ctx context.Context, id string) (ballchasing.Replay, []byte, error)
}

// Config connects the queue to the rest of the app.
type Config struct {
	Store      *store.Store
	API        func() API      // nil while no token is set
	Dirs       func() []string // replay folders
	Visibility func() string
	// Processed runs once ballchasing has processed an upload. duplicate
	// means the replay was on ballchasing already.
	Processed func(ctx context.Context, f store.File, r ballchasing.Replay, raw []byte, duplicate bool)
	Failed    func(f store.File) // failed for good or rejected
	Changed   func()             // any state changed, for the interface
}

// Status is the state of the whole queue.
type Status struct {
	Paused   bool `json:"paused"`
	Offline  bool `json:"offline"`   // ballchasing not reachable, waiting
	BadToken bool `json:"bad_token"` // stopped until the token is fixed
}

// Queue uploads replay files and tracks them in the store.
type Queue struct {
	cfg Config
	now func() time.Time

	paused   atomic.Bool
	offline  atomic.Bool
	badToken atomic.Bool
	wake     chan struct{}

	mu      sync.Mutex // guards pending
	pending map[string]int64

	retryDelays []time.Duration // after the first and second failed attempt
	offlineWait time.Duration
	pollFast    time.Duration
	pollSlow    time.Duration
}

const (
	maxAttempts = 3
	maxPolls    = 140 // about an hour of waiting for processing
)

func New(cfg Config) *Queue {
	return &Queue{
		cfg: cfg, now: time.Now, wake: make(chan struct{}, 1), pending: map[string]int64{},
		retryDelays: []time.Duration{time.Minute, 5 * time.Minute},
		offlineWait: 30 * time.Second,
		pollFast:    3 * time.Second,
		pollSlow:    30 * time.Second,
	}
}

// Run scans the folders and works through the queue until ctx is done.
func (q *Queue) Run(ctx context.Context) {
	scan := time.NewTicker(3 * time.Second)
	tick := time.NewTicker(time.Second)
	defer scan.Stop()
	defer tick.Stop()
	for {
		for q.step(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-scan.C:
			q.scan()
		case <-tick.C:
		case <-q.wake:
		}
	}
}

// Wake makes the queue look for work right away.
func (q *Queue) Wake() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) changed() {
	if q.cfg.Changed != nil {
		q.cfg.Changed()
	}
}

func (q *Queue) Status() Status {
	return Status{Paused: q.paused.Load(), Offline: q.offline.Load(), BadToken: q.badToken.Load()}
}

// SetPaused stops or resumes watching and uploading. Files saved during
// the pause are picked up afterwards.
func (q *Queue) SetPaused(p bool) {
	q.paused.Store(p)
	q.changed()
	q.Wake()
}

// TokenChanged lets a stopped queue try again with a new token.
func (q *Queue) TokenChanged() {
	q.badToken.Store(false)
	q.changed()
	q.Wake()
}

func isReplay(name string) bool {
	// bc-*.replay are downloads for watching, never uploaded again
	return strings.EqualFold(filepath.Ext(name), ".replay") && !strings.HasPrefix(name, "bc-")
}

// scan queues every new replay file once its size has stopped changing.
func (q *Queue) scan() {
	if q.paused.Load() {
		return
	}
	known, err := q.cfg.Store.FileNames()
	if err != nil {
		log.Printf("Status lesen: %v", err)
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	added := false
	for _, dir := range q.cfg.Dirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // e.g. DemosEpic on a Steam install
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !isReplay(name) || known[name] {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(dir, name)
			size, seen := q.pending[path]
			if !seen || size != info.Size() || info.Size() == 0 {
				q.pending[path] = info.Size() // new or still being written
				continue
			}
			delete(q.pending, path)
			if err := q.cfg.Store.PutFile(store.File{Name: name, Dir: dir, State: store.Queued, NextTry: q.now()}); err != nil {
				log.Printf("Status speichern: %v", err)
				continue
			}
			log.Printf("Neues Replay %s", name)
			added = true
		}
	}
	if added {
		q.changed()
		q.Wake()
	}
}

// Seed registers the files already in the folders without uploading them,
// so the first start does not upload the whole backlog.
func (q *Queue) Seed() (int, error) {
	known, err := q.cfg.Store.FileNames()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, dir := range q.cfg.Dirs() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !isReplay(e.Name()) || known[e.Name()] {
				continue
			}
			if err := q.cfg.Store.PutFile(store.File{Name: e.Name(), Dir: dir, State: store.Seeded}); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// locate returns the path of a file, looking through the replay folders if
// its folder is unknown (files taken over from older versions).
func (q *Queue) locate(f store.File) (string, bool) {
	if f.Dir != "" {
		p := filepath.Join(f.Dir, f.Name)
		_, err := os.Stat(p)
		return p, err == nil
	}
	for _, dir := range q.cfg.Dirs() {
		p := filepath.Join(dir, f.Name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// QueueSeeded queues registered old replays saved since the given time (all
// if zero) and returns how many.
func (q *Queue) QueueSeeded(since time.Time) (int, error) {
	files, err := q.cfg.Store.Files(store.Seeded)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range files {
		path, ok := q.locate(f)
		if !ok {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.ModTime().Before(since) {
			continue
		}
		f.Dir, f.State, f.NextTry = filepath.Dir(path), store.Queued, q.now()
		if err := q.cfg.Store.PutFile(f); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		q.changed()
		q.Wake()
	}
	return n, nil
}

// CountSeeded returns how many registered old replays exist since the
// given time.
func (q *Queue) CountSeeded(since time.Time) int {
	files, _ := q.cfg.Store.Files(store.Seeded)
	n := 0
	for _, f := range files {
		if path, ok := q.locate(f); ok {
			if info, err := os.Stat(path); err == nil && !info.ModTime().Before(since) {
				n++
			}
		}
	}
	return n
}

// AddManual queues a file picked by hand.
func (q *Queue) AddManual(path string) error {
	f := store.File{Name: filepath.Base(path), Dir: filepath.Dir(path), State: store.Queued, NextTry: q.now(), Manual: true}
	if old, ok, err := q.cfg.Store.File(f.Name); err != nil {
		return err
	} else if ok {
		f.Added = old.Added
	}
	if err := q.cfg.Store.PutFile(f); err != nil {
		return err
	}
	q.changed()
	q.Wake()
	return nil
}

// Retry queues a failed file again with fresh attempts.
func (q *Queue) Retry(name string) error {
	f, ok, err := q.cfg.Store.File(name)
	if err != nil || !ok {
		return err
	}
	switch f.State {
	case store.Failed, store.Rejected, store.Retry:
	default:
		return nil
	}
	f.State, f.Attempts, f.Error, f.NextTry = store.Queued, 0, "", q.now()
	if err := q.cfg.Store.PutFile(f); err != nil {
		return err
	}
	q.changed()
	q.Wake()
	return nil
}

// RetryAll queues all failed and waiting files again.
func (q *Queue) RetryAll() (int, error) {
	files, err := q.cfg.Store.Files(store.Failed, store.Retry)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		if err := q.Retry(f.Name); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// step handles the next due file and reports whether there was one.
func (q *Queue) step(ctx context.Context) bool {
	if ctx.Err() != nil || q.paused.Load() || q.badToken.Load() || q.cfg.API == nil {
		return false
	}
	api := q.cfg.API()
	if api == nil {
		return false
	}
	files, err := q.cfg.Store.Files(store.Uploading, store.Queued, store.Retry, store.Processing)
	if err != nil {
		log.Printf("Status lesen: %v", err)
		return false
	}
	now := q.now()
	for _, f := range files {
		if f.NextTry.After(now) {
			continue
		}
		if f.State == store.Processing {
			q.poll(ctx, api, f)
		} else {
			q.upload(ctx, api, f)
		}
		return true
	}
	return false
}

func (q *Queue) put(f store.File) {
	if err := q.cfg.Store.PutFile(f); err != nil {
		log.Printf("Status speichern: %v", err)
	}
	q.changed()
}

func (q *Queue) upload(ctx context.Context, api API, f store.File) {
	path, ok := q.locate(f)
	if !ok {
		f.State, f.Error = store.Failed, "Datei nicht mehr vorhanden"
		q.put(f)
		q.fail(f)
		return
	}
	f.State, f.Dir = store.Uploading, filepath.Dir(path)
	q.put(f)

	id, duplicate, err := api.Upload(ctx, path, q.cfg.Visibility())
	now := q.now()
	switch {
	case ctx.Err() != nil:
		f.State = store.Queued
		q.put(f)
	case err == nil:
		q.offline.Store(false)
		f.State, f.ReplayID, f.Error, f.Attempts, f.NextTry, f.Duplicate = store.Processing, id, "", 0, now, duplicate
		q.put(f)
	case errors.Is(err, ballchasing.ErrRejected):
		f.State, f.Error = store.Rejected, err.Error()
		log.Printf("%s abgelehnt: %v", f.Name, err)
		q.put(f)
		q.fail(f)
	case errors.Is(err, ballchasing.ErrBadToken):
		f.State = store.Queued
		q.badToken.Store(true)
		log.Print("Der ballchasing-Token ist ungültig, Uploads warten auf einen neuen Token")
		q.put(f)
	case errors.Is(err, fs.ErrNotExist):
		f.State, f.Error = store.Failed, "Datei nicht mehr vorhanden"
		q.put(f)
		q.fail(f)
	case ballchasing.Unavailable(err):
		if !q.offline.Swap(true) {
			log.Printf("ballchasing nicht erreichbar (%v), Uploads warten", err)
		}
		f.State, f.Error, f.NextTry = store.Queued, "ballchasing nicht erreichbar", now.Add(q.offlineWait)
		q.put(f)
	default:
		q.offline.Store(false)
		f.Attempts++
		f.Error = err.Error()
		if f.Attempts >= maxAttempts {
			f.State = store.Failed
			log.Printf("Upload von %s endgültig fehlgeschlagen: %v", f.Name, err)
			q.put(f)
			q.fail(f)
			return
		}
		wait := q.retryDelays[min(f.Attempts-1, len(q.retryDelays)-1)]
		f.State, f.NextTry = store.Retry, now.Add(wait)
		log.Printf("Upload von %s fehlgeschlagen, neuer Versuch in %s: %v", f.Name, wait, err)
		q.put(f)
	}
}

// poll asks ballchasing whether an upload is processed.
func (q *Queue) poll(ctx context.Context, api API, f store.File) {
	r, raw, err := api.GetReplay(ctx, f.ReplayID)
	now := q.now()
	switch {
	case ctx.Err() != nil:
		return
	case errors.Is(err, ballchasing.ErrBadToken):
		q.badToken.Store(true)
		q.changed()
		return
	case err != nil && ballchasing.Unavailable(err):
		q.offline.Store(true)
		f.NextTry = now.Add(q.offlineWait)
		q.put(f)
		return
	case err != nil:
		f.State, f.Error = store.Failed, "Abruf nach dem Upload: "+err.Error()
		q.put(f)
		q.fail(f)
		return
	}
	q.offline.Store(false)
	switch r.Status {
	case "ok":
		f.State, f.Error, f.Attempts = store.Done, "", 0
		if f.Duplicate {
			log.Printf("%s war schon auf ballchasing: https://ballchasing.com/replay/%s", f.Name, f.ReplayID)
		} else {
			log.Printf("%s hochgeladen: https://ballchasing.com/replay/%s", f.Name, f.ReplayID)
		}
		q.put(f)
		if q.cfg.Processed != nil {
			q.cfg.Processed(ctx, f, r, raw, f.Duplicate)
		}
	case "failed":
		f.State, f.Error = store.Rejected, "ballchasing konnte das Replay nicht verarbeiten"
		q.put(f)
		q.fail(f)
	default: // pending
		f.Attempts++
		if f.Attempts >= maxPolls {
			f.State, f.Error = store.Failed, "ballchasing verarbeitet das Replay nicht"
			q.put(f)
			q.fail(f)
			return
		}
		wait := q.pollFast
		if f.Attempts > 20 {
			wait = q.pollSlow
		}
		f.NextTry = now.Add(wait)
		q.put(f)
	}
}

func (q *Queue) fail(f store.File) {
	if q.cfg.Failed != nil {
		q.cfg.Failed(f)
	}
}
