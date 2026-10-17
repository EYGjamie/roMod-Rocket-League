package app

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// borrowName is the file name of a replay downloaded for watching. The
// upload queue skips "bc-" files.
func borrowName(id string) string { return "bc-" + id + ".replay" }

// Borrow downloads a replay into the replay folder so the game lists it.
// It is deleted again after watching unless kept.
func (a *App) Borrow(ctx context.Context, id string) (string, error) {
	c := a.currentClient()
	if c == nil {
		return "", errors.New("nicht mit ballchasing verbunden")
	}
	dir := ""
	for _, d := range a.demoDirs() {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dir = d
			break
		}
	}
	if dir == "" {
		dirs := a.demoDirs()
		if len(dirs) == 0 {
			return "", errors.New("kein Replay-Ordner eingestellt")
		}
		dir = dirs[0]
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	name := borrowName(id)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	err = c.Download(ctx, id, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := a.store.PutBorrow(store.Borrow{Name: name, ReplayID: id}); err != nil {
		return "", err
	}
	log.Printf("%s fürs Anschauen im Spiel geladen", name)
	a.events.publish("replays")
	return name, nil
}

// KeepBorrow keeps a downloaded replay (or lets it be cleaned up again).
func (a *App) KeepBorrow(id string, keep bool) error {
	err := a.store.PutBorrow(store.Borrow{Name: borrowName(id), ReplayID: id, Keep: keep})
	a.events.publish("replays")
	return err
}

// cleanBorrows deletes the downloaded replays after the player left a
// replay in the game.
func (a *App) cleanBorrows() {
	a.removeBorrows(func(b store.Borrow) bool { return !b.Keep })
}

// cleanOldBorrows removes leftovers from earlier runs.
func (a *App) cleanOldBorrows() {
	cut := a.now().Add(-24 * time.Hour)
	a.removeBorrows(func(b store.Borrow) bool { return !b.Keep && b.Added.Before(cut) })
}

func (a *App) removeBorrows(match func(store.Borrow) bool) {
	borrows, _ := a.store.Borrows()
	removed := 0
	for _, b := range borrows {
		if !match(b) {
			continue
		}
		for _, d := range a.demoDirs() {
			if err := os.Remove(filepath.Join(d, b.Name)); err == nil {
				removed++
			}
		}
		a.store.DeleteBorrow(b.Name)
	}
	if removed > 0 {
		log.Printf("%d fürs Anschauen geladene Replays wieder gelöscht", removed)
		a.events.publish("replays")
	}
}

// borrowed returns the IDs of downloaded replays and whether they are kept.
func (a *App) borrowed() map[string]bool {
	borrows, _ := a.store.Borrows()
	out := map[string]bool{}
	for _, b := range borrows {
		out[b.ReplayID] = b.Keep
	}
	return out
}
