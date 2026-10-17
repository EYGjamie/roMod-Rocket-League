package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// errRejected marks uploads that will never succeed, e.g. corrupt replays.
var errRejected = errors.New("von ballchasing abgelehnt")

var errBadToken = errors.New("ballchasing-Token ungültig")

type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string {
	return strings.TrimSpace(fmt.Sprintf("HTTP %d %s", e.status, e.msg))
}

func hasStatus(err error, status int) bool {
	var se *statusError
	return errors.As(err, &se) && se.status == status
}

func errorText(body []byte) string {
	var r struct {
		Error string `json:"error"`
	}
	json.Unmarshal(body, &r)
	return r.Error
}

type ballchasing struct {
	baseURL    string
	token      string
	visibility string
	group      string
	client     *http.Client
}

// Field names follow the ballchasing API, the GUI receives them unchanged.
type replay struct {
	ID       string     `json:"id"`
	Title    string     `json:"replay_title"`
	Date     string     `json:"date"`
	Map      string     `json:"map_name"`
	Playlist string     `json:"playlist_name"`
	Duration float64    `json:"duration"`
	Blue     team       `json:"blue"`
	Orange   team       `json:"orange"`
	Groups   []groupRef `json:"groups"`
}

type team struct {
	Name    string `json:"name"`
	Goals   int    `json:"goals"`
	Players []struct {
		Name string `json:"name"`
	} `json:"players"`
}

type groupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type replayPage struct {
	List []replay `json:"list"`
	Next string   `json:"next,omitempty"`
}

type group struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       string `json:"created"`
	DirectReplays int    `json:"direct_replays"`
}

// do sends req with the token and returns status and body.
func (b *ballchasing) do(req *http.Request) (int, []byte, error) {
	req.Header.Set("Authorization", b.token)
	resp, err := b.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, body, errBadToken
	}
	return resp.StatusCode, body, nil
}

// call sends in as JSON (if not nil) and decodes the reply into out (if not
// nil). Any status other than want is an error.
func (b *ballchasing) call(ctx context.Context, method, u string, in, out any, want int) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	status, data, err := b.do(req)
	if err != nil {
		return err
	}
	if status != want {
		return &statusError{status, errorText(data)}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ping checks the token and returns the account name.
func (b *ballchasing) ping(ctx context.Context) (string, error) {
	var r struct {
		Name string `json:"name"`
	}
	err := b.call(ctx, http.MethodGet, b.baseURL+"/api/", nil, &r, http.StatusOK)
	return r.Name, err
}

// upload sends one replay file. A replay that is already on ballchasing
// counts as success and returns the existing ID.
func (b *ballchasing) upload(ctx context.Context, path string) (id string, duplicate bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", false, err
	}
	part.Write(data)
	if err := mw.Close(); err != nil {
		return "", false, err
	}

	q := url.Values{"visibility": {b.visibility}}
	if b.group != "" {
		q.Set("group", b.group)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/api/v2/upload?"+q.Encode(), &body)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	status, resp, err := b.do(req)
	if err != nil {
		return "", false, err
	}
	var r struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp, &r)
	switch status {
	case http.StatusCreated:
		return r.ID, false, nil
	case http.StatusConflict:
		return r.ID, true, nil
	case http.StatusBadRequest:
		return "", false, fmt.Errorf("%w: %s", errRejected, errorText(resp))
	default:
		return "", false, &statusError{status, errorText(resp)}
	}
}

// listReplays returns the account's own replays, newest first. next is the
// pagination link from a previous page, or empty for the first page.
func (b *ballchasing) listReplays(ctx context.Context, next string) (replayPage, error) {
	u := b.baseURL + "/api/replays?" + url.Values{
		"uploader": {"me"},
		"count":    {"50"},
		"sort-by":  {"replay-date"},
		"sort-dir": {"desc"},
	}.Encode()
	if next != "" {
		if !strings.HasPrefix(next, b.baseURL+"/api/replays?") {
			return replayPage{}, errors.New("ungültiger Link für die nächste Seite")
		}
		u = next
	}
	var p replayPage
	err := b.call(ctx, http.MethodGet, u, nil, &p, http.StatusOK)
	return p, err
}

// listGroups returns the groups created by the account, newest first.
func (b *ballchasing) listGroups(ctx context.Context) ([]group, error) {
	u := b.baseURL + "/api/groups?" + url.Values{
		"creator":  {"me"},
		"count":    {"200"},
		"sort-by":  {"created"},
		"sort-dir": {"desc"},
	}.Encode()
	var r struct {
		List []group `json:"list"`
	}
	err := b.call(ctx, http.MethodGet, u, nil, &r, http.StatusOK)
	return r.List, err
}

// createGroup creates a top-level group. clusters selects team detection
// that tolerates substitutes.
func (b *ballchasing) createGroup(ctx context.Context, name string, clusters bool) (string, error) {
	teams := "by-distinct-players"
	if clusters {
		teams = "by-player-clusters"
	}
	in := map[string]string{
		"name":                  name,
		"player_identification": "by-id",
		"team_identification":   teams,
	}
	var r struct {
		ID string `json:"id"`
	}
	err := b.call(ctx, http.MethodPost, b.baseURL+"/api/groups", in, &r, http.StatusCreated)
	return r.ID, err
}

// setGroup moves a replay into a group.
func (b *ballchasing) setGroup(ctx context.Context, replayID, groupID string) error {
	u := b.baseURL + "/api/replays/" + url.PathEscape(replayID)
	return b.call(ctx, http.MethodPatch, u, map[string]string{"group": groupID}, nil, http.StatusNoContent)
}
