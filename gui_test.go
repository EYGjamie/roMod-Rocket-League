package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

type testGUI struct {
	*gui
	api *fakeAPI
	srv *httptest.Server
}

func newTestGUI(t *testing.T) *testGUI {
	t.Helper()
	api := fakeBallchasing(t)
	g := newGUI(api.client())
	g.pace, g.retry = 0, 0
	srv := httptest.NewServer(g.handler())
	t.Cleanup(srv.Close)
	g.addr = srv.Listener.Addr().String()
	return &testGUI{gui: g, api: api, srv: srv}
}

// do sends a request to the GUI and decodes a JSON reply into out.
func (tg *testGUI) do(t *testing.T, method, path, token, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, tg.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-Token", token)
	}
	resp, err := tg.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v in %s", method, path, err, data)
		}
	}
	return resp.StatusCode
}

func TestGUIRejectsForeignRequests(t *testing.T) {
	tg := newTestGUI(t)

	if code := tg.do(t, "GET", "/api/groups", "", "", nil); code != http.StatusForbidden {
		t.Errorf("without token: status %d, want 403", code)
	}
	if code := tg.do(t, "GET", "/api/groups", "falsch", "", nil); code != http.StatusForbidden {
		t.Errorf("wrong token: status %d, want 403", code)
	}

	req, _ := http.NewRequest("GET", tg.srv.URL+"/api/groups", nil)
	req.Host = "rebind.example:80" // DNS rebinding
	req.Header.Set("X-Token", tg.token)
	resp, err := tg.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Host: status %d, want 403", resp.StatusCode)
	}

	resp, err = tg.srv.Client().Get(tg.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(page), "<title>Replays gruppieren</title>") {
		t.Errorf("index: status %d", resp.StatusCode)
	}
}

func TestGUIListsReplaysAndGroups(t *testing.T) {
	tg := newTestGUI(t)

	var page replayPage
	if code := tg.do(t, "GET", "/api/replays", tg.token, "", &page); code != http.StatusOK || len(page.List) != 1 {
		t.Fatalf("replays: status %d, page %+v", code, page)
	}
	var page2 replayPage
	if code := tg.do(t, "GET", "/api/replays?next="+url.QueryEscape(page.Next), tg.token, "", &page2); code != http.StatusOK || page2.List[0].ID != "r2" {
		t.Errorf("next page: status %d, page %+v", code, page2)
	}

	var groups []group
	if code := tg.do(t, "GET", "/api/groups", tg.token, "", &groups); code != http.StatusOK || len(groups) != 1 || groups[0].DirectReplays != 3 {
		t.Errorf("groups: status %d, %+v", code, groups)
	}
}

func TestGUICreatesGroupAndAssigns(t *testing.T) {
	tg := newTestGUI(t)

	var created struct{ ID string }
	if code := tg.do(t, "POST", "/api/groups", tg.token, `{"name":" Liga 1 ","clusters":true}`, &created); code != http.StatusCreated || created.ID != "neu-1" {
		t.Fatalf("create: status %d, %+v", code, created)
	}
	if code := tg.do(t, "POST", "/api/groups", tg.token, `{"name":"  "}`, nil); code != http.StatusBadRequest {
		t.Errorf("empty name: status %d, want 400", code)
	}

	var res struct {
		Assigned int
		Failed   []assignFailure
	}
	body := `{"group":"neu-1","replays":["r1","r-limit","r-gone"]}`
	if code := tg.do(t, "POST", "/api/assign", tg.token, body, &res); code != http.StatusOK {
		t.Fatalf("assign: status %d", code)
	}
	if res.Assigned != 2 || len(res.Failed) != 1 || res.Failed[0].ID != "r-gone" {
		t.Errorf("assign result = %+v, want 2 assigned and r-gone failed", res)
	}
	if got := tg.api.patched("r-limit"); !slices.Equal(got, []string{"neu-1", "neu-1"}) {
		t.Errorf("r-limit patched %v, want retry after 429", got)
	}
	if got := tg.api.patched("r1"); !slices.Equal(got, []string{"neu-1"}) {
		t.Errorf("r1 patched %v", got)
	}
}
