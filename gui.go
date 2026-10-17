package main

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
)

//go:embed gui.html
var guiHTML []byte

// gui serves the local page for sorting replays into ballchasing groups.
// It only answers requests addressed to its own 127.0.0.1 address that carry
// the per-run token, so other websites in the browser cannot use it.
type gui struct {
	bc    *ballchasing
	token string
	addr  string        // host:port the page is served on
	pace  time.Duration // pause between replay updates (rate limit 2/s)
	retry time.Duration // wait after hitting the rate limit
}

func newGUI(bc *ballchasing) *gui {
	b := make([]byte, 16)
	rand.Read(b)
	return &gui{bc: bc, token: hex.EncodeToString(b), pace: 600 * time.Millisecond, retry: 2 * time.Second}
}

// start listens on a free local port until ctx is done and returns the URL
// to open.
func (g *gui) start(ctx context.Context) (string, error) {
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

func (g *gui) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", g.index)
	mux.HandleFunc("GET /api/replays", g.replays)
	mux.HandleFunc("GET /api/groups", g.groups)
	mux.HandleFunc("POST /api/groups", g.createGroup)
	mux.HandleFunc("POST /api/assign", g.assign)
	return g.guard(mux)
}

func (g *gui) guard(next http.Handler) http.Handler {
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
	if errors.Is(err, errBadToken) {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (g *gui) index(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
	w.Write(guiHTML)
}

func (g *gui) replays(w http.ResponseWriter, r *http.Request) {
	page, err := g.bc.listReplays(r.Context(), r.URL.Query().Get("next"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if page.List == nil {
		page.List = []replay{}
	}
	writeJSON(w, http.StatusOK, page)
}

func (g *gui) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := g.bc.listGroups(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if groups == nil {
		groups = []group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (g *gui) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Clusters bool   `json:"clusters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Gruppenname fehlt"})
		return
	}
	id, err := g.bc.createGroup(r.Context(), strings.TrimSpace(in.Name), in.Clusters)
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
func (g *gui) assign(w http.ResponseWriter, r *http.Request) {
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
		err := g.bc.setGroup(ctx, id, in.Group)
		if hasStatus(err, http.StatusTooManyRequests) && sleepCtx(ctx, g.retry) == nil {
			err = g.bc.setGroup(ctx, id, in.Group)
		}
		if err != nil {
			failed = append(failed, assignFailure{id, err.Error()})
		}
	}
	n := len(in.Replays) - len(failed)
	log.Printf("%d Replays in Gruppe https://ballchasing.com/group/%s verschoben, %d fehlgeschlagen", n, in.Group, len(failed))
	writeJSON(w, http.StatusOK, map[string]any{"assigned": n, "failed": failed})
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
