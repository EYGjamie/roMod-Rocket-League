// Package ballchasing is a client for the parts of the ballchasing.com API
// this tool uses: uploading replays and sorting them into groups.
package ballchasing

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

// ErrRejected marks uploads that will never succeed, e.g. corrupt replays.
var ErrRejected = errors.New("von ballchasing abgelehnt")

var ErrBadToken = errors.New("ballchasing-Token ungültig")

type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string {
	return strings.TrimSpace(fmt.Sprintf("HTTP %d %s", e.status, e.msg))
}

// HasStatus reports whether err is an API reply with the given HTTP status.
func HasStatus(err error, status int) bool {
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

type Client struct {
	BaseURL    string
	Token      string
	Visibility string // for uploads: public, unlisted or private
	Group      string // group ID for uploads, may be empty
	HTTPClient *http.Client
}

// Field names follow the ballchasing API, the GUI receives them unchanged.
type Replay struct {
	ID       string     `json:"id"`
	Title    string     `json:"replay_title"`
	Date     string     `json:"date"`
	Map      string     `json:"map_name"`
	Playlist string     `json:"playlist_name"`
	Duration float64    `json:"duration"`
	Blue     Team       `json:"blue"`
	Orange   Team       `json:"orange"`
	Groups   []GroupRef `json:"groups"`
}

type Team struct {
	Name    string `json:"name"`
	Goals   int    `json:"goals"`
	Players []struct {
		Name string `json:"name"`
	} `json:"players"`
}

type GroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ReplayPage struct {
	List []Replay `json:"list"`
	Next string   `json:"next,omitempty"`
}

type Group struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       string `json:"created"`
	DirectReplays int    `json:"direct_replays"`
}

// do sends req with the token and returns status and body.
func (b *Client) do(req *http.Request) (int, []byte, error) {
	req.Header.Set("Authorization", b.Token)
	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, body, ErrBadToken
	}
	return resp.StatusCode, body, nil
}

// call sends in as JSON (if not nil) and decodes the reply into out (if not
// nil). Any status other than want is an error.
func (b *Client) call(ctx context.Context, method, u string, in, out any, want int) error {
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

// Ping checks the token and returns the account name.
func (b *Client) Ping(ctx context.Context) (string, error) {
	var r struct {
		Name string `json:"name"`
	}
	err := b.call(ctx, http.MethodGet, b.BaseURL+"/api/", nil, &r, http.StatusOK)
	return r.Name, err
}

// Upload sends one replay file. A replay that is already on ballchasing
// counts as success and returns the existing ID.
func (b *Client) Upload(ctx context.Context, path string) (id string, duplicate bool, err error) {
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

	q := url.Values{"visibility": {b.Visibility}}
	if b.Group != "" {
		q.Set("group", b.Group)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+"/api/v2/upload?"+q.Encode(), &body)
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
		return "", false, fmt.Errorf("%w: %s", ErrRejected, errorText(resp))
	default:
		return "", false, &statusError{status, errorText(resp)}
	}
}

// ListReplays returns the account's own replays, newest first. next is the
// pagination link from a previous page, or empty for the first page.
func (b *Client) ListReplays(ctx context.Context, next string) (ReplayPage, error) {
	u := b.BaseURL + "/api/replays?" + url.Values{
		"uploader": {"me"},
		"count":    {"50"},
		"sort-by":  {"replay-date"},
		"sort-dir": {"desc"},
	}.Encode()
	if next != "" {
		if !strings.HasPrefix(next, b.BaseURL+"/api/replays?") {
			return ReplayPage{}, errors.New("ungültiger Link für die nächste Seite")
		}
		u = next
	}
	var p ReplayPage
	err := b.call(ctx, http.MethodGet, u, nil, &p, http.StatusOK)
	return p, err
}

// ListGroups returns the groups created by the account, newest first. If
// parent is not empty, only its direct subgroups are returned.
func (b *Client) ListGroups(ctx context.Context, parent string) ([]Group, error) {
	q := url.Values{
		"creator":  {"me"},
		"count":    {"200"},
		"sort-by":  {"created"},
		"sort-dir": {"desc"},
	}
	if parent != "" {
		q.Set("group", parent)
	}
	var r struct {
		List []Group `json:"list"`
	}
	err := b.call(ctx, http.MethodGet, b.BaseURL+"/api/groups?"+q.Encode(), nil, &r, http.StatusOK)
	return r.List, err
}

// CreateGroup creates a group, as a subgroup of parent if that is not empty.
// clusters selects team detection that tolerates substitutes.
func (b *Client) CreateGroup(ctx context.Context, name, parent string, clusters bool) (string, error) {
	teams := "by-distinct-players"
	if clusters {
		teams = "by-player-clusters"
	}
	in := map[string]string{
		"name":                  name,
		"player_identification": "by-id",
		"team_identification":   teams,
	}
	if parent != "" {
		in["parent"] = parent
	}
	var r struct {
		ID string `json:"id"`
	}
	err := b.call(ctx, http.MethodPost, b.BaseURL+"/api/groups", in, &r, http.StatusCreated)
	return r.ID, err
}

// SetGroup moves a replay into a group.
func (b *Client) SetGroup(ctx context.Context, replayID, groupID string) error {
	u := b.BaseURL + "/api/replays/" + url.PathEscape(replayID)
	return b.call(ctx, http.MethodPatch, u, map[string]string{"group": groupID}, nil, http.StatusNoContent)
}
