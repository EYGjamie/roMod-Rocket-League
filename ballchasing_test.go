package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// fakeAPI imitates the parts of the ballchasing API this tool uses.
type fakeAPI struct {
	*httptest.Server
	mu      sync.Mutex
	patches map[string][]string // replay ID -> group IDs it was patched to
}

func fakeBallchasing(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{patches: map[string][]string{}}
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
		reply(w, http.StatusOK, `{"list":[{"id":"g1","name":"Liga","created":"2026-10-01T10:00:00Z","direct_replays":3}]}`)
	})
	mux.HandleFunc("POST /api/groups", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["name"] == "" || in["player_identification"] != "by-id" || in["team_identification"] != "by-player-clusters" {
			reply(w, http.StatusBadRequest, `{"error":"bad group"}`)
			return
		}
		reply(w, http.StatusCreated, `{"id":"neu-1","link":"https://ballchasing.com/api/groups/neu-1"}`)
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

func (f *fakeAPI) patched(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.patches[id])
}

func (f *fakeAPI) client() *ballchasing {
	return &ballchasing{baseURL: f.URL, token: "tok", visibility: "private", group: "grp", client: f.Client()}
}

func TestBallchasingUpload(t *testing.T) {
	bc := fakeBallchasing(t).client()
	ctx := context.Background()
	dir := t.TempDir()

	tests := []struct {
		file          string
		wantID        string
		wantDuplicate bool
		wantErr       error
	}{
		{file: "new.replay", wantID: "abc"},
		{file: "dup.replay", wantID: "old", wantDuplicate: true},
		{file: "bad.replay", wantErr: errRejected},
		{file: "busy.replay", wantErr: errors.New("any")},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			if err := os.WriteFile(path, []byte("replay-bytes"), 0o644); err != nil {
				t.Fatal(err)
			}
			id, dup, err := bc.upload(ctx, path)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("upload() error = %v", err)
			case tc.wantErr != nil && err == nil:
				t.Fatal("upload() error = nil, want error")
			case errors.Is(tc.wantErr, errRejected) && !errors.Is(err, errRejected):
				t.Fatalf("upload() error = %v, want errRejected", err)
			case !errors.Is(tc.wantErr, errRejected) && errors.Is(err, errRejected):
				t.Fatalf("upload() error = %v, must stay retryable", err)
			}
			if id != tc.wantID || dup != tc.wantDuplicate {
				t.Errorf("upload() = %q, %v, want %q, %v", id, dup, tc.wantID, tc.wantDuplicate)
			}
		})
	}
}

func TestBallchasingPing(t *testing.T) {
	bc := fakeBallchasing(t).client()
	ctx := context.Background()

	if name, err := bc.ping(ctx); err != nil || name != "Jamie" {
		t.Errorf("ping() = %q, %v, want Jamie, nil", name, err)
	}

	bc.token = "falsch"
	if _, err := bc.ping(ctx); !errors.Is(err, errBadToken) {
		t.Errorf("ping() error = %v, want errBadToken", err)
	}
}

func TestBallchasingListReplays(t *testing.T) {
	bc := fakeBallchasing(t).client()
	ctx := context.Background()

	page, err := bc.listReplays(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 1 || page.Next == "" {
		t.Fatalf("first page = %+v, want one replay and a next link", page)
	}
	r := page.List[0]
	if r.Blue.Goals != 3 || r.Orange.Goals != 1 || r.Blue.Players[0].Name != "Jamie" || r.Groups[0].Name != "Liga" {
		t.Errorf("replay decoded as %+v", r)
	}

	page, err = bc.listReplays(ctx, page.Next)
	if err != nil || len(page.List) != 1 || page.List[0].ID != "r2" || page.Next != "" {
		t.Errorf("second page = %+v, %v", page, err)
	}

	if _, err := bc.listReplays(ctx, "https://example.com/api/replays?x=1"); err == nil {
		t.Error("listReplays() followed a foreign next link")
	}
}
