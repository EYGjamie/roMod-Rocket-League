// Package ballchasing is a client for the parts of the ballchasing.com API
// this tool uses: uploading replays, reading them back and sorting them into
// groups.
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
	"strconv"
	"strings"
	"time"
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

// Unavailable reports whether err means ballchasing could not be reached or
// is overloaded, as opposed to rejecting the request.
func Unavailable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		switch se.status {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	var ue *url.Error // transport errors: offline, DNS, timeouts
	return errors.As(err, &ue) && !errors.Is(err, context.Canceled)
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
	HTTPClient *http.Client
	// Unpaced turns off the spacing between calls that keeps the client below
	// the API rate limits, for tests.
	Unpaced bool

	limit limiter
}

// Minimum spacing between calls per endpoint, just below the limits for
// accounts without Patreon (2 calls per second, downloads 1 per second).
const (
	paceDefault  = 550 * time.Millisecond
	paceDownload = 1050 * time.Millisecond
)

// do sends req with the token and returns status and body. A rate-limited
// request is repeated twice after a pause.
func (b *Client) do(req *http.Request, endpoint string) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		resp, err := b.send(req, endpoint)
		if err != nil {
			return 0, nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return 0, nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 2 && req.GetBody != nil {
			if err := sleep(req.Context(), retryAfter(resp, attempt)); err != nil {
				return 0, nil, err
			}
			if req.Body, err = req.GetBody(); err != nil {
				return 0, nil, err
			}
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return resp.StatusCode, body, ErrBadToken
		}
		return resp.StatusCode, body, nil
	}
}

func (b *Client) send(req *http.Request, endpoint string) (*http.Response, error) {
	if !b.Unpaced {
		pace := paceDefault
		if endpoint == "download" {
			pace = paceDownload
		}
		if err := b.limit.wait(req.Context(), endpoint, pace); err != nil {
			return nil, err
		}
	}
	req.Header.Set("Authorization", b.Token)
	return b.HTTPClient.Do(req)
}

func retryAfter(resp *http.Response, attempt int) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 && s < 60 {
		return time.Duration(s) * time.Second
	}
	return time.Duration(2+3*attempt) * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// call sends in as JSON (if not nil) and decodes the reply into out (if not
// nil). Any status other than want is an error.
func (b *Client) call(ctx context.Context, endpoint, method, u string, in, out any, want int) error {
	var data []byte
	if in != nil {
		var err error
		if data, err = json.Marshal(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	status, body, err := b.do(req, endpoint)
	if err != nil {
		return err
	}
	if status != want {
		return &statusError{status, errorText(body)}
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Me is the account the token belongs to.
type Me struct {
	Name    string `json:"name"`
	SteamID string `json:"steam_id"`
	Type    string `json:"type"` // patron tier: regular, gold, diamond, champion, gc
	Quota   Quota  `json:"quota"`
}

// Quota is what the account may still do.
type Quota struct {
	Uploads Usage `json:"uploads_in_24h"`
}

// Usage counts calls in a window. Max 0 means ballchasing sent no limit.
type Usage struct {
	Used int `json:"used"`
	Max  int `json:"max"`
}

// Ping checks the token and returns the account.
func (b *Client) Ping(ctx context.Context) (Me, error) {
	var me Me
	err := b.call(ctx, "ping", http.MethodGet, b.BaseURL+"/api/", nil, &me, http.StatusOK)
	return me, err
}

// Upload sends one replay file. A replay that is already on ballchasing
// counts as success and returns the existing ID.
func (b *Client) Upload(ctx context.Context, path, visibility string) (id string, duplicate bool, err error) {
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
	payload := body.Bytes()

	q := url.Values{"visibility": {visibility}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+"/api/v2/upload?"+q.Encode(), bytes.NewReader(payload))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	status, resp, err := b.do(req, "upload")
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

// GetReplay returns a replay with all statistics, decoded and as the raw
// JSON reply. Status is "pending" until ballchasing has processed it.
func (b *Client) GetReplay(ctx context.Context, id string) (Replay, []byte, error) {
	var raw json.RawMessage
	err := b.call(ctx, "replay", http.MethodGet, b.BaseURL+"/api/replays/"+url.PathEscape(id), nil, &raw, http.StatusOK)
	if err != nil {
		return Replay{}, nil, err
	}
	var r Replay
	err = json.Unmarshal(raw, &r)
	return r, raw, err
}

// ReplayQuery filters the replay list. Uploader "me" lists the account's own
// uploads, PlayerID ("steam:7656…", "epic:…") every replay with that player.
type ReplayQuery struct {
	Uploader     string
	PlayerID     string
	CreatedAfter time.Time
	Count        int
	Oldest       bool // oldest uploads first instead of newest
}

// ListReplays returns one page of replays. next is the pagination link from
// a previous page, or empty for the first page.
func (b *Client) ListReplays(ctx context.Context, q ReplayQuery, next string) (ReplayPage, error) {
	v := url.Values{"sort-by": {"upload-date"}, "sort-dir": {"desc"}, "count": {"200"}}
	if q.Oldest {
		v.Set("sort-dir", "asc")
	}
	if q.Count > 0 {
		v.Set("count", strconv.Itoa(q.Count))
	}
	if q.Uploader != "" {
		v.Set("uploader", q.Uploader)
	}
	if q.PlayerID != "" {
		v.Set("player-id", q.PlayerID)
	}
	if !q.CreatedAfter.IsZero() {
		v.Set("created-after", q.CreatedAfter.UTC().Format(time.RFC3339))
	}
	u := b.BaseURL + "/api/replays?" + v.Encode()
	if next != "" {
		if !strings.HasPrefix(next, b.BaseURL+"/api/replays?") {
			return ReplayPage{}, errors.New("ungültiger Link für die nächste Seite")
		}
		u = next
	}
	var p ReplayPage
	err := b.call(ctx, "list", http.MethodGet, u, nil, &p, http.StatusOK)
	return p, err
}

// ListGroups returns the groups created by the account, newest first. If
// parent is empty these are only the top-level groups, otherwise the direct
// subgroups of parent.
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
	err := b.call(ctx, "groups", http.MethodGet, b.BaseURL+"/api/groups?"+q.Encode(), nil, &r, http.StatusOK)
	return r.List, err
}

// GetGroup returns a group with the statistics ballchasing computes for its
// players and teams, as the raw JSON reply. Its status is "ok", or
// "pending_replays" while replays in it are still processed.
func (b *Client) GetGroup(ctx context.Context, id string) ([]byte, error) {
	var raw json.RawMessage
	err := b.call(ctx, "group", http.MethodGet, b.BaseURL+"/api/groups/"+url.PathEscape(id), nil, &raw, http.StatusOK)
	return raw, err
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
	err := b.call(ctx, "groups-write", http.MethodPost, b.BaseURL+"/api/groups", in, &r, http.StatusCreated)
	return r.ID, err
}

// SetParent moves a group with its replays and subgroups into another
// group, or to the top level if parent is empty.
func (b *Client) SetParent(ctx context.Context, id, parent string) error {
	u := b.BaseURL + "/api/groups/" + url.PathEscape(id)
	return b.call(ctx, "groups-write", http.MethodPatch, u, map[string]string{"parent": parent}, nil, http.StatusNoContent)
}

// DeleteGroup deletes a group on ballchasing.
func (b *Client) DeleteGroup(ctx context.Context, id string) error {
	return b.call(ctx, "groups-write", http.MethodDelete, b.BaseURL+"/api/groups/"+url.PathEscape(id), nil, nil, http.StatusNoContent)
}

// SetGroup moves a replay into a group, or out of every group if groupID is
// empty.
func (b *Client) SetGroup(ctx context.Context, replayID, groupID string) error {
	return b.patchReplay(ctx, replayID, map[string]string{"group": groupID})
}

// SetVisibility makes a replay public, unlisted or private.
func (b *Client) SetVisibility(ctx context.Context, replayID, visibility string) error {
	return b.patchReplay(ctx, replayID, map[string]string{"visibility": visibility})
}

func (b *Client) patchReplay(ctx context.Context, replayID string, fields map[string]string) error {
	u := b.BaseURL + "/api/replays/" + url.PathEscape(replayID)
	return b.call(ctx, "replay-write", http.MethodPatch, u, fields, nil, http.StatusNoContent)
}

// DeleteReplay deletes a replay on ballchasing for good.
func (b *Client) DeleteReplay(ctx context.Context, replayID string) error {
	return b.call(ctx, "replay-write", http.MethodDelete, b.BaseURL+"/api/replays/"+url.PathEscape(replayID), nil, nil, http.StatusNoContent)
}

// Download writes the replay file to w.
func (b *Client) Download(ctx context.Context, replayID string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.BaseURL+"/api/replays/"+url.PathEscape(replayID)+"/file", nil)
	if err != nil {
		return err
	}
	resp, err := b.send(req, "download")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrBadToken
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return &statusError{resp.StatusCode, errorText(body)}
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
