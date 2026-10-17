// Package gui serves the page for sorting replays into ballchasing groups
// and shows it in an overlay window.
package gui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
)

//go:embed gui.html
var guiHTML []byte

// Server serves the local page for sorting replays into ballchasing groups.
// It only answers requests addressed to its own 127.0.0.1 address that carry
// the per-run token, so other websites in the browser cannot use it.
type Server struct {
	bc    *ballchasing.Client
	token string
	addr  string        // host:port the page is served on
	pace  time.Duration // pause between replay updates (rate limit 2/s)
	retry time.Duration // wait after hitting the rate limit
}

func NewServer(bc *ballchasing.Client) *Server {
	b := make([]byte, 16)
	rand.Read(b)
	return &Server{bc: bc, token: hex.EncodeToString(b), pace: 600 * time.Millisecond, retry: 2 * time.Second}
}

// Start listens on a free local port until ctx is done and returns the URL
// to open.
func (g *Server) Start(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	g.addr = ln.Addr().String()
	srv := &http.Server{Handler: g.handler(), ReadHeaderTimeout: 10 * time.Second}
	context.AfterFunc(ctx, func() { srv.Close() })
	go func() {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Oberfläche beendet: %v", err)
		}
	}()
	return "http://" + g.addr + "/?k=" + g.token, nil
}

func (g *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", g.index)
	mux.HandleFunc("GET /api/replays", g.replays)
	mux.HandleFunc("GET /api/groups", g.groups)
	mux.HandleFunc("POST /api/groups", g.createGroup)
	mux.HandleFunc("POST /api/assign", g.assign)
	return g.guard(mux)
}

func (g *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != g.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(g.token)) != 1 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Zugriff verweigert, Oberfläche über den Hotkey neu öffnen"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, ballchasing.ErrBadToken) {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (g *Server) index(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
	w.Write(guiHTML)
}

func (g *Server) replays(w http.ResponseWriter, r *http.Request) {
	page, err := g.bc.ListReplays(r.Context(), r.URL.Query().Get("next"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if page.List == nil {
		page.List = []ballchasing.Replay{}
	}
	writeJSON(w, http.StatusOK, page)
}

// groups lists the account's groups, or the subgroups of ?parent=.
func (g *Server) groups(w http.ResponseWriter, r *http.Request) {
	var groups []ballchasing.Group
	err := g.retryLimited(r.Context(), func() (err error) {
		groups, err = g.bc.ListGroups(r.Context(), r.URL.Query().Get("parent"))
		return err
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if groups == nil {
		groups = []ballchasing.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (g *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Parent   string `json:"parent"` // empty for a top-level group
		Clusters bool   `json:"clusters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Gruppenname fehlt"})
		return
	}
	id, err := g.bc.CreateGroup(r.Context(), strings.TrimSpace(in.Name), in.Parent, in.Clusters)
	if err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("Gruppe %q angelegt: https://ballchasing.com/group/%s", in.Name, id)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

type assignFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// assign moves the replays into the group one by one, paced to stay below
// the API rate limit.
func (g *Server) assign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Group   string   `json:"group"`
		Replays []string `json:"replays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Group == "" || len(in.Replays) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Gruppe oder Replays fehlen"})
		return
	}
	ctx := r.Context()
	failed := []assignFailure{}
	for i, id := range in.Replays {
		if i > 0 && sleepCtx(ctx, g.pace) != nil {
			return
		}
		err := g.retryLimited(ctx, func() error { return g.bc.SetGroup(ctx, id, in.Group) })
		if err != nil {
			failed = append(failed, assignFailure{id, err.Error()})
		}
	}
	n := len(in.Replays) - len(failed)
	log.Printf("%d Replays in Gruppe https://ballchasing.com/group/%s verschoben, %d fehlgeschlagen", n, in.Group, len(failed))
	writeJSON(w, http.StatusOK, map[string]any{"assigned": n, "failed": failed})
}

// retryLimited calls f once more after a pause if ballchasing answered with
// its rate limit.
func (g *Server) retryLimited(ctx context.Context, f func() error) error {
	err := f()
	if ballchasing.HasStatus(err, http.StatusTooManyRequests) && sleepCtx(ctx, g.retry) == nil {
		err = f()
	}
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
