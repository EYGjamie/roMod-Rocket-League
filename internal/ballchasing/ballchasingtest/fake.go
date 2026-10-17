// Package ballchasingtest provides a fake ballchasing API for tests.
package ballchasingtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
)

// Server imitates the parts of the ballchasing API this tool uses.
type Server struct {
	*httptest.Server
	mu      sync.Mutex
	patches map[string][]string // replay ID -> group IDs it was patched to
}

func New(t *testing.T) *Server {
	t.Helper()
	f := &Server{patches: map[string][]string{}}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, status int, body string) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}

	mux.HandleFunc("GET /api/{$}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, `{"name":"Jamie","type":"regular"}`)
	})
	mux.HandleFunc("POST /api/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("visibility") != "private" || r.URL.Query().Get("group") != "grp" {
			t.Errorf("unexpected upload query %s", r.URL.RawQuery)
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			return
		}
		if b, _ := io.ReadAll(file); string(b) != "replay-bytes" {
			t.Errorf("uploaded content = %q", b)
		}
		switch hdr.Filename {
		case "new.replay":
			reply(w, http.StatusCreated, `{"id":"abc","location":"https://ballchasing.com/replay/abc"}`)
		case "dup.replay":
			reply(w, http.StatusConflict, `{"id":"old","error":"duplicate replay"}`)
		case "bad.replay":
			reply(w, http.StatusBadRequest, `{"error":"failed to parse replay"}`)
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	})
	mux.HandleFunc("GET /api/replays", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uploader") != "me" {
			t.Errorf("replays query %s without uploader=me", r.URL.RawQuery)
		}
		if r.URL.Query().Get("after") == "p2" {
			reply(w, http.StatusOK, `{"count":2,"list":[{"id":"r2","replay_title":"Zwei","duration":301.5}]}`)
			return
		}
		reply(w, http.StatusOK, `{"count":2,"next":"`+f.URL+`/api/replays?after=p2&uploader=me","list":[
			{"id":"r1","replay_title":"Eins","date":"2026-10-17T20:15:00+02:00","map_name":"DFH Stadium",
			 "playlist_name":"Ranked Doubles","duration":300,
			 "blue":{"name":"Blau","goals":3,"players":[{"name":"Jamie","id":{"platform":"epic","id":"x"}}]},
			 "orange":{"goals":1,"players":[{"name":"Gegner"}]},
			 "groups":[{"id":"g1","name":"Liga","link":"https://ballchasing.com/api/groups/g1"}]}]}`)
	})
	mux.HandleFunc("GET /api/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("creator") != "me" {
			t.Errorf("groups query %s without creator=me", r.URL.RawQuery)
		}
		// g1 has the subgroup g1-s1, which has none
		switch r.URL.Query().Get("group") {
		case "":
			reply(w, http.StatusOK, `{"list":[{"id":"g1","name":"Liga","created":"2026-10-01T10:00:00Z","direct_replays":3}]}`)
		case "g1":
			reply(w, http.StatusOK, `{"list":[{"id":"g1-s1","name":"Spieltag 1","created":"2026-10-08T10:00:00Z","direct_replays":2}]}`)
		default:
			reply(w, http.StatusOK, `{"list":[]}`)
		}
	})
	mux.HandleFunc("POST /api/groups", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		parent, hasParent := in["parent"]
		if in["name"] == "" || in["player_identification"] != "by-id" || in["team_identification"] != "by-player-clusters" ||
			hasParent && parent == "" {
			reply(w, http.StatusBadRequest, `{"error":"bad group"}`)
			return
		}
		switch parent {
		case "":
			reply(w, http.StatusCreated, `{"id":"neu-1","link":"https://ballchasing.com/api/groups/neu-1"}`)
		case "g1":
			reply(w, http.StatusCreated, `{"id":"unter-1","link":"https://ballchasing.com/api/groups/unter-1"}`)
		default:
			reply(w, http.StatusBadRequest, `{"error":"parent group not found"}`)
		}
	})
	mux.HandleFunc("PATCH /api/replays/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		id := r.PathValue("id")
		f.mu.Lock()
		f.patches[id] = append(f.patches[id], in["group"])
		n := len(f.patches[id])
		f.mu.Unlock()
		switch {
		case id == "r-gone":
			reply(w, http.StatusNotFound, `{"error":"replay not found"}`)
		case id == "r-limit" && n == 1:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "tok" {
			reply(w, http.StatusUnauthorized, `{"error":"invalid API key"}`)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// Patched returns the group IDs the replay was moved to, in order.
func (f *Server) Patched(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.patches[id])
}

// API returns a client for this server with the token it accepts.
func (f *Server) API() *ballchasing.Client {
	return &ballchasing.Client{BaseURL: f.URL, Token: "tok", Visibility: "private", Group: "grp", HTTPClient: f.Client()}
}
