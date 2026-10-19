package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/config"
	"github.com/EYGjamie/auto-replay-upload/internal/game"
	"github.com/EYGjamie/auto-replay-upload/internal/rules"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
	"github.com/EYGjamie/auto-replay-upload/internal/web"
)

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.UI, "ui")
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("GET /{$}", a.index)
	mux.Handle("GET /assets/", http.StripPrefix("/assets", files))

	mux.HandleFunc("GET /api/events", a.events.serve)
	mux.HandleFunc("GET /api/state", a.apiState)
	mux.HandleFunc("GET /api/overview", a.apiOverview)
	mux.HandleFunc("POST /api/pause", a.apiPause)
	mux.HandleFunc("GET /api/queue", a.apiQueue)
	mux.HandleFunc("POST /api/queue/retry", a.apiRetry)
	mux.HandleFunc("POST /api/queue/retry-all", a.apiRetryAll)
	mux.HandleFunc("POST /api/upload", a.apiUpload)
	mux.HandleFunc("POST /api/open-folder", a.apiOpenFolder)
	mux.HandleFunc("POST /api/open-log", a.apiOpenLog)

	mux.HandleFunc("GET /api/replays", a.apiReplays)
	mux.HandleFunc("GET /api/replays/{id}", a.apiReplay)
	mux.HandleFunc("POST /api/replays/move", a.apiMove)
	mux.HandleFunc("POST /api/replays/visibility", a.apiVisibility)
	mux.HandleFunc("POST /api/replays/delete", a.apiDeleteReplays)
	mux.HandleFunc("POST /api/replays/{id}/borrow", a.apiBorrow)
	mux.HandleFunc("POST /api/replays/{id}/keep", a.apiKeep)
	mux.HandleFunc("POST /api/replays/{id}/reveal", a.apiReveal)
	mux.HandleFunc("GET /api/stats", a.apiStats)

	mux.HandleFunc("GET /api/groups", a.apiGroups)
	mux.HandleFunc("POST /api/groups", a.apiCreateGroup)
	mux.HandleFunc("DELETE /api/groups/{id}", a.apiDeleteGroup)
	mux.HandleFunc("POST /api/sync", a.apiSync)
	mux.HandleFunc("GET /api/inbox", a.apiInbox)
	mux.HandleFunc("GET /api/suggestions", a.apiSuggestions)
	mux.HandleFunc("POST /api/suggestions/apply", a.apiApplySuggestion)
	mux.HandleFunc("POST /api/suggestions/ignore", a.apiIgnore)

	mux.HandleFunc("GET /api/rules", a.apiRules)
	mux.HandleFunc("POST /api/rules", a.apiSaveRule)
	mux.HandleFunc("PUT /api/rules/{id}", a.apiSaveRule)
	mux.HandleFunc("DELETE /api/rules/{id}", a.apiDeleteRule)
	mux.HandleFunc("POST /api/rules/order", a.apiOrderRules)
	mux.HandleFunc("POST /api/rule-suggestions/accept", a.apiAcceptRule)
	mux.HandleFunc("POST /api/rule-suggestions/ignore", a.apiIgnore)

	mux.HandleFunc("GET /api/settings", a.apiSettings)
	mux.HandleFunc("PUT /api/settings", a.apiUpdateSettings)
	mux.HandleFunc("POST /api/token", a.apiSetToken)
	mux.HandleFunc("DELETE /api/token", a.apiRemoveToken)
	mux.HandleFunc("POST /api/autostart", a.apiAutostart)
	mux.HandleFunc("POST /api/test-key", a.apiTestKey)
	mux.HandleFunc("GET /api/identities", a.apiIdentities)
	mux.HandleFunc("PUT /api/identities", a.apiSetIdentities)

	mux.HandleFunc("GET /api/setup", a.apiSetup)
	mux.HandleFunc("POST /api/setup/rules", a.apiSetupRules)
	mux.HandleFunc("POST /api/setup/backfill", a.apiBackfill)
	mux.HandleFunc("POST /api/setup/done", a.apiSetupDone)
	return a.guard(mux)
}

// guard only answers requests to our own 127.0.0.1 address (no DNS
// rebinding) and API calls carrying the per-run key, so other websites in
// the browser cannot use the interface.
func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != a.ln.Addr().String() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			key := r.Header.Get("X-Token")
			if r.URL.Path == "/api/events" {
				key = r.URL.Query().Get("k") // EventSource cannot set headers
			}
			if subtle.ConstantTimeCompare([]byte(key), []byte(a.token)) != 1 {
				writeJSON(w, http.StatusForbidden, errBody("Zugriff verweigert, Oberfläche über das Symbol neu öffnen"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) index(w http.ResponseWriter, r *http.Request) {
	page, err := web.UI.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
	w.Write(page)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

// fail answers with an error; ballchasing problems become 502, a bad token
// 401.
func fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ballchasing.ErrBadToken):
		status = http.StatusUnauthorized
	case ballchasing.Unavailable(err), ballchasing.HasStatus(err, http.StatusNotFound), ballchasing.HasStatus(err, http.StatusInternalServerError):
		status = http.StatusBadGateway
	}
	writeJSON(w, status, errBody(err.Error()))
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("Ungültige Anfrage: "+err.Error()))
		return false
	}
	return true
}

func ok(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }

// --- state and overview ---

type stateView struct {
	Version   string       `json:"version"`
	Platform  Platform     `json:"platform"`
	SetupDone bool         `json:"setup_done"`
	Account   accountView  `json:"account"`
	StatsAPI  bool         `json:"stats_api"`
	InMatch   bool         `json:"in_match"`
	Paused    pausedView   `json:"paused"`
	Queue     queueSummary `json:"queue"`
	Inbox     int          `json:"inbox"`
	RuleTips  int          `json:"rule_tips"`
	Syncing   bool         `json:"syncing"`
	Me        []string     `json:"me"`
}

type accountView struct {
	Status  string            `json:"status"` // none, ok, invalid, offline
	Name    string            `json:"name"`
	FromEnv bool              `json:"from_env"`
	Tier    string            `json:"tier"`
	Uploads ballchasing.Usage `json:"uploads_24h"`
}

type pausedView struct {
	Watch bool `json:"watch"`
	Mark  bool `json:"mark"`
}

type queueSummary struct {
	Active   int  `json:"active"`
	Failed   int  `json:"failed"`
	Offline  bool `json:"offline"`
	BadToken bool `json:"bad_token"`
}

func (a *App) accountInfo() accountView {
	a.mu.RLock()
	defer a.mu.RUnlock()
	st := a.account
	if a.client == nil {
		st = "none"
	}
	return accountView{Status: st, Name: a.me.Name, FromEnv: a.cfg.TokenFromEnv(), Tier: a.me.Type, Uploads: a.me.Quota.Uploads}
}

func (a *App) queueSummary() queueSummary {
	counts, _ := a.store.CountFiles()
	qs := a.queue.Status()
	return queueSummary{
		Active:   counts[store.Queued] + counts[store.Uploading] + counts[store.Processing] + counts[store.Retry],
		Failed:   counts[store.Failed] + counts[store.Rejected],
		Offline:  qs.Offline,
		BadToken: qs.BadToken,
	}
}

func (a *App) inboxReplays() []store.Replay {
	var out []store.Replay
	for _, r := range a.visible() {
		if r.Own && r.GroupID == "" && r.Status != "failed" {
			out = append(out, r)
		}
	}
	return out
}

// Version is set by main for the interface.
var Version = "dev"

func (a *App) apiState(w http.ResponseWriter, r *http.Request) {
	watch, mark := a.pausedState()
	decisions, _ := a.store.Decisions()
	ruleList, _ := a.store.Rules()
	tips := rules.SuggestRules(decisions, ruleList, a.store.Ignored)
	var me []string
	for pid := range a.myIDs() {
		me = append(me, pid)
	}
	sort.Strings(me)
	acc := a.accountInfo()
	if qs := a.queue.Status(); qs.BadToken {
		acc.Status = "invalid"
	}
	writeJSON(w, http.StatusOK, stateView{
		Version: Version, Platform: a.desk.Platform(), SetupDone: a.config().SetupDone, Account: acc, StatsAPI: a.statsAPI.Load(),
		InMatch: a.tracker.InMatch(), Paused: pausedView{watch, mark}, Queue: a.queueSummary(),
		Inbox: len(a.inboxReplays()), RuleTips: len(tips), Syncing: a.Syncing(), Me: me,
	})
}

func (a *App) apiOverview(w http.ResponseWriter, r *http.Request) {
	vc := a.newViewCtx()
	var recent []replayView
	own := a.visible()
	sort.SliceStable(own, func(i, j int) bool { return own[i].Created.After(own[j].Created) })
	for _, rep := range own {
		if rep.Own && len(recent) < 8 {
			recent = append(recent, vc.replay(rep))
		}
	}
	session, has := a.CurrentSession()
	out := map[string]any{"recent": recent, "session": nil}
	if has {
		out["session"] = session
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) apiPause(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Watch *bool `json:"watch"`
		Mark  *bool `json:"mark"`
	}
	if !decode(w, r, &in) {
		return
	}
	watch, mark := a.pausedState()
	if in.Watch != nil {
		watch = *in.Watch
	}
	if in.Mark != nil {
		mark = *in.Mark
	}
	a.SetPaused(watch, mark)
	ok(w)
}

// --- queue ---

func (a *App) apiQueue(w http.ResponseWriter, r *http.Request) {
	files, err := a.store.Files(store.Queued, store.Uploading, store.Processing, store.Retry, store.Failed, store.Rejected)
	if err != nil {
		fail(w, err)
		return
	}
	items := []fileView{}
	for i := len(files) - 1; i >= 0; i-- { // newest first
		items = append(items, toFileView(files[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": a.queue.Status(), "items": items})
}

func (a *App) apiRetry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := a.queue.Retry(in.Name); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiRetryAll(w http.ResponseWriter, r *http.Request) {
	files, _ := a.store.Files(store.Rejected)
	for _, f := range files {
		a.queue.Retry(f.Name)
	}
	n, err := a.queue.RetryAll()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": n + len(files)})
}

// apiUpload stores files sent from the page and queues them.
func (a *App) apiUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		fail(w, err)
		return
	}
	dir := filepath.Join(a.dataDir, "manuell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(w, err)
		return
	}
	n := 0
	for _, fh := range r.MultipartForm.File["files"] {
		name := filepath.Base(fh.Filename)
		if !strings.EqualFold(filepath.Ext(name), ".replay") || strings.HasPrefix(name, "bc-") {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			fail(w, err)
			return
		}
		path := filepath.Join(dir, name)
		dst, err := os.Create(path)
		if err == nil {
			_, err = io.Copy(dst, src)
			if cerr := dst.Close(); err == nil {
				err = cerr
			}
		}
		src.Close()
		if err != nil {
			fail(w, err)
			return
		}
		if err := a.queue.AddManual(path); err != nil {
			fail(w, err)
			return
		}
		n++
	}
	log.Printf("%d Replays zum Hochladen hinzugefügt", n)
	writeJSON(w, http.StatusOK, map[string]int{"queued": n})
}

// UploadFiles queues files picked in the tray's file dialog.
func (a *App) UploadFiles(paths []string) {
	for _, p := range paths {
		if err := a.queue.AddManual(p); err != nil {
			log.Printf("%s: %v", p, err)
		}
	}
}

func (a *App) apiOpenFolder(w http.ResponseWriter, r *http.Request) {
	a.OpenReplayFolder()
	ok(w)
}

// OpenReplayFolder shows the first existing replay folder in the file
// manager.
func (a *App) OpenReplayFolder() {
	if d := a.ReplayFolder(); d != "" {
		a.desk.OpenFolder(d)
	}
}

// ReplayFolder returns the first replay folder that exists, or "".
func (a *App) ReplayFolder() string {
	for _, d := range a.demoDirs() {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

func (a *App) apiOpenLog(w http.ResponseWriter, r *http.Request) {
	if err := a.desk.Edit(a.opts.LogPath); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

// --- replays ---

func (a *App) apiReplays(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days, _ := strconv.Atoi(q.Get("days"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	writeJSON(w, http.StatusOK, a.list(listQuery{Q: q.Get("q"), Status: q.Get("status"), Playlist: q.Get("playlist"),
		Group: q.Get("group"), Origin: q.Get("origin"), Days: days, Offset: offset, Limit: limit}))
}

// apiReplay returns a replay with all statistics, fetching them if needed.
func (a *App) apiReplay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rep, found, err := a.store.Replay(id)
	if err != nil || !found {
		writeJSON(w, http.StatusNotFound, errBody("Replay nicht gefunden"))
		return
	}
	if !rep.HasDetail {
		if err := a.fetchDetail(r.Context(), id); err != nil {
			fail(w, err)
			return
		}
		rep, _, _ = a.store.Replay(id)
	}
	raw, _ := a.store.Detail(id)
	var others []replayView
	vc := a.newViewCtx()
	all, _ := a.store.Replays()
	for _, o := range all {
		if o.DupOf == id {
			others = append(others, vc.replay(o))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"replay": vc.replay(rep), "detail": json.RawMessage(nonEmpty(raw)), "also": others})
}

func nonEmpty(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	return raw
}

type batchResult struct {
	Done   int      `json:"done"`
	Errors []string `json:"errors"`
}

func (a *App) apiMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs   []string `json:"ids"`
		Group string   `json:"group"`
	}
	if !decode(w, r, &in) {
		return
	}
	err := a.Move(r.Context(), in.IDs, in.Group, true)
	res := batchResult{Done: len(in.IDs), Errors: []string{}}
	if err != nil {
		res.Errors = strings.Split(err.Error(), "; ")
		res.Done -= len(res.Errors)
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *App) apiVisibility(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs        []string `json:"ids"`
		Visibility string   `json:"visibility"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Visibility != "public" && in.Visibility != "unlisted" && in.Visibility != "private" {
		writeJSON(w, http.StatusBadRequest, errBody("Sichtbarkeit ungültig"))
		return
	}
	a.batch(w, r.Context(), in.IDs, func(c *ballchasing.Client, id string) error {
		if err := c.SetVisibility(r.Context(), id, in.Visibility); err != nil {
			return err
		}
		return a.store.SetVisibility(id, in.Visibility)
	})
}

func (a *App) apiDeleteReplays(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.batch(w, r.Context(), in.IDs, func(c *ballchasing.Client, id string) error {
		rep, _, _ := a.store.Replay(id)
		if err := c.DeleteReplay(r.Context(), id); err != nil {
			return err
		}
		a.adjustCounts(rep.GroupID, "")
		log.Printf("Replay %s auf ballchasing gelöscht", id)
		return a.store.DeleteReplay(id)
	})
	a.dedupe()
}

// batch runs fn for each own replay and reports the failures.
func (a *App) batch(w http.ResponseWriter, ctx context.Context, ids []string, fn func(*ballchasing.Client, string) error) {
	c := a.currentClient()
	if c == nil {
		writeJSON(w, http.StatusBadRequest, errBody("nicht mit ballchasing verbunden"))
		return
	}
	res := batchResult{Errors: []string{}}
	for _, id := range ids {
		rep, found, _ := a.store.Replay(id)
		switch {
		case !found:
			res.Errors = append(res.Errors, id+": nicht gefunden")
		case !rep.Own:
			res.Errors = append(res.Errors, id+": nur eigene Uploads lassen sich ändern")
		default:
			if err := fn(c, id); err != nil {
				res.Errors = append(res.Errors, id+": "+err.Error())
				continue
			}
			res.Done++
		}
	}
	a.events.publish("replays")
	a.events.publish("groups")
	a.events.publish("inbox")
	writeJSON(w, http.StatusOK, res)
}

func (a *App) apiBorrow(w http.ResponseWriter, r *http.Request) {
	name, err := a.Borrow(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (a *App) apiKeep(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Keep bool `json:"keep"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := a.KeepBorrow(r.PathValue("id"), in.Keep); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiReveal(w http.ResponseWriter, r *http.Request) {
	rep, found, _ := a.store.Replay(r.PathValue("id"))
	if !found || rep.File == "" {
		writeJSON(w, http.StatusNotFound, errBody("keine lokale Datei"))
		return
	}
	f, found, _ := a.store.File(rep.File)
	if !found || f.Deleted {
		writeJSON(w, http.StatusNotFound, errBody("Datei wurde gelöscht"))
		return
	}
	if err := a.desk.Reveal(filepath.Join(f.Dir, f.Name)); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiStats(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	writeJSON(w, http.StatusOK, a.Stats(days))
}

// --- folders, inbox, suggestions ---

func (a *App) apiGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.store.Groups()
	if err != nil {
		fail(w, err)
		return
	}
	if groups == nil {
		groups = []store.Group{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups, "unsorted": len(a.inboxReplays()), "syncing": a.Syncing(),
		"synced": a.store.Meta("groups_synced")})
}

func (a *App) apiCreateGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Parent   string `json:"parent"`
		Clusters bool   `json:"clusters"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("Ordnername fehlt"))
		return
	}
	g, err := a.CreateGroup(r.Context(), strings.TrimSpace(in.Name), in.Parent, in.Clusters)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

// apiDeleteGroup only deletes empty folders: what ballchasing does with
// the replays of a deleted group is not documented.
func (a *App) apiDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	groups, _ := a.store.Groups()
	for _, g := range groups {
		if g.ID == id && (g.Indirect > 0 || g.Direct > 0) {
			writeJSON(w, http.StatusConflict, errBody("Nur leere Ordner lassen sich löschen"))
			return
		}
		if g.Parent == id {
			writeJSON(w, http.StatusConflict, errBody("Der Ordner hat noch Unterordner"))
			return
		}
	}
	c := a.currentClient()
	if c == nil {
		writeJSON(w, http.StatusBadRequest, errBody("nicht mit ballchasing verbunden"))
		return
	}
	if err := c.DeleteGroup(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	a.store.DeleteGroup(id)
	a.events.publish("groups")
	ok(w)
}

func (a *App) apiSync(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Full bool `json:"full"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	a.requestSync(in.Full)
	ok(w)
}

type inboxItem struct {
	Replay      replayView         `json:"replay"`
	Suggestions []rules.Suggestion `json:"suggestions"`
}

type splitView struct {
	PlaylistID string   `json:"playlist_id"`
	Playlist   string   `json:"playlist"`
	Count      int      `json:"count"`
	GroupID    string   `json:"group_id"`
	Group      string   `json:"group"`
	IDs        []string `json:"ids"`
}

type inboxSession struct {
	Start  time.Time   `json:"start"`
	End    time.Time   `json:"end"`
	Wins   int         `json:"wins"`
	Losses int         `json:"losses"`
	Items  []inboxItem `json:"items"`
	Split  []splitView `json:"split"`
}

// apiInbox lists the unsorted own uploads by session, each with
// suggestions, and per session a proposal to split it by playlist.
func (a *App) apiInbox(w http.ResponseWriter, r *http.Request) {
	vc := a.newViewCtx()
	sessions := sessionsOf(a.visible(), vc.me)
	sessionOf := func(t time.Time) int {
		for i, s := range sessions {
			if !t.Before(s.Start) && !t.After(s.End) {
				return i
			}
		}
		return -1
	}
	byIndex := map[int]*inboxSession{}
	var order []int
	for _, rep := range a.inboxReplays() { // newest first
		i := sessionOf(rep.Date)
		s := byIndex[i]
		if s == nil {
			s = &inboxSession{Start: rep.Date, End: rep.Date}
			byIndex[i] = s
			order = append(order, i)
		}
		if rep.Date.Before(s.Start) {
			s.Start = rep.Date
		}
		switch rep.Result(vc.me) {
		case "win":
			s.Wins++
		case "loss":
			s.Losses++
		}
		s.Items = append(s.Items, inboxItem{Replay: vc.replay(rep), Suggestions: a.suggestions(rep, 4)})
	}
	out := []inboxSession{}
	for _, i := range order {
		s := byIndex[i]
		s.Split = a.split(s.Items)
		out = append(out, *s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// split proposes one folder per playlist for a set of replays.
func (a *App) split(items []inboxItem) []splitView {
	byPlaylist := map[string]*splitView{}
	var order []string
	for _, it := range items {
		sv := byPlaylist[it.Replay.PlaylistID]
		if sv == nil {
			sv = &splitView{PlaylistID: it.Replay.PlaylistID, Playlist: it.Replay.Playlist}
			if len(it.Suggestions) > 0 {
				sv.GroupID, sv.Group = it.Suggestions[0].GroupID, it.Suggestions[0].Name
			}
			byPlaylist[it.Replay.PlaylistID] = sv
			order = append(order, it.Replay.PlaylistID)
		}
		sv.Count++
		sv.IDs = append(sv.IDs, it.Replay.ID)
	}
	out := []splitView{}
	for _, p := range order {
		out = append(out, *byPlaylist[p])
	}
	return out
}

type folderTip struct {
	Key     string   `json:"key"`
	Text    string   `json:"text"`
	GroupID string   `json:"group_id"`
	IDs     []string `json:"ids"`
}

// folderTips are the suggestions above the unsorted replays, e.g. "4
// private Spiele → Privat & Training".
func (a *App) folderTips() []folderTip {
	vc := a.newViewCtx()
	var items []inboxItem
	for _, rep := range a.inboxReplays() {
		items = append(items, inboxItem{Replay: vc.replay(rep), Suggestions: a.suggestions(rep, 1)})
	}
	var out []folderTip
	for _, sv := range a.split(items) {
		key := "ordner:" + sv.PlaylistID + "|" + sv.GroupID
		if sv.GroupID == "" || sv.Count < 2 || a.store.Ignored(key) {
			continue
		}
		out = append(out, folderTip{Key: key, GroupID: sv.GroupID, IDs: sv.IDs,
			Text: fmt.Sprintf("%d %s-Spiele → %s", sv.Count, sv.Playlist, sv.Group)})
	}
	return out
}

func (a *App) apiSuggestions(w http.ResponseWriter, r *http.Request) {
	tips := a.folderTips()
	if tips == nil {
		tips = []folderTip{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tips": tips})
}

func (a *App) apiApplySuggestion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &in) {
		return
	}
	for _, t := range a.folderTips() {
		if t.Key == in.Key {
			if err := a.Move(r.Context(), t.IDs, t.GroupID, true); err != nil {
				fail(w, err)
				return
			}
			ok(w)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, errBody("Vorschlag nicht mehr aktuell"))
}

func (a *App) apiIgnore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.store.Ignore(in.Key)
	a.events.publish("rules")
	a.events.publish("inbox")
	ok(w)
}

// --- rules ---

type ruleView struct {
	store.Rule
	Target      string   `json:"target"`       // folder path
	PlaylistTxt []string `json:"playlist_txt"` // display names
	PlayerTxt   []string `json:"player_txt"`
}

type ruleTipView struct {
	rules.RuleSuggestion
	Text string `json:"text"`
}

func (a *App) apiRules(w http.ResponseWriter, r *http.Request) {
	ruleList, _ := a.store.Rules()
	groups, _ := a.store.Groups()
	names := a.playerNames()
	views := []ruleView{}
	for _, rule := range ruleList {
		v := ruleView{Rule: rule, Target: a.groupPath(groups, rule.TargetID)}
		if v.Target == "" {
			v.Target = rule.TargetName
		}
		for _, p := range rule.Playlists {
			v.PlaylistTxt = append(v.PlaylistTxt, rules.PlaylistName(p))
		}
		for _, p := range rule.Players {
			v.PlayerTxt = append(v.PlayerTxt, firstNonEmpty(names[p], p))
		}
		views = append(views, v)
	}
	decisions, _ := a.store.Decisions()
	tips := []ruleTipView{}
	for _, s := range rules.SuggestRules(decisions, ruleList, a.store.Ignored) {
		text := fmt.Sprintf("%d× %s", s.Count, rules.PlaylistName(s.PlaylistID))
		if len(s.Mates) > 0 {
			var mates []string
			for _, m := range s.Mates {
				mates = append(mates, firstNonEmpty(names[m], m))
			}
			text += " mit " + strings.Join(mates, " und ")
		}
		text += " → " + a.groupPath(groups, s.GroupID)
		tips = append(tips, ruleTipView{RuleSuggestion: s, Text: text})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": views, "tips": tips, "playlists": rules.Playlists, "players": a.knownMates()})
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

type mateView struct {
	PID   string `json:"pid"`
	Name  string `json:"name"`
	Games int    `json:"games"`
}

// knownMates lists the players the user played together with, most games
// first, for rule conditions.
func (a *App) knownMates() []mateView {
	me := a.myIDs()
	names := a.playerNames()
	count := map[string]int{}
	for _, r := range a.visible() {
		for _, m := range r.Mates(me) {
			count[m]++
		}
	}
	out := []mateView{}
	for pid, n := range count {
		out = append(out, mateView{PID: pid, Name: firstNonEmpty(names[pid], pid), Games: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Games != out[j].Games {
			return out[i].Games > out[j].Games
		}
		return out[i].Name < out[j].Name
	})
	return out[:min(len(out), 50)]
}

func (a *App) apiSaveRule(w http.ResponseWriter, r *http.Request) {
	var rule store.Rule
	if !decode(w, r, &rule) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		rule.ID, _ = strconv.ParseInt(id, 10, 64)
	}
	if len(rule.Playlists) == 0 && len(rule.Players) == 0 {
		writeJSON(w, http.StatusBadRequest, errBody("Die Regel braucht eine Playlist oder einen Mitspieler"))
		return
	}
	if rule.TargetID == "" && strings.TrimSpace(rule.TargetName) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("Zielordner fehlt"))
		return
	}
	if err := a.store.PutRule(&rule); err != nil {
		fail(w, err)
		return
	}
	a.events.publish("rules")
	writeJSON(w, http.StatusOK, rule)
}

func (a *App) apiDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := a.store.DeleteRule(id); err != nil {
		fail(w, err)
		return
	}
	a.events.publish("rules")
	ok(w)
}

func (a *App) apiOrderRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := a.store.OrderRules(in.IDs); err != nil {
		fail(w, err)
		return
	}
	a.events.publish("rules")
	ok(w)
}

// apiAcceptRule turns a suggestion into a rule placed before all others,
// since it is more specific than the playlist rules.
func (a *App) apiAcceptRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &in) {
		return
	}
	decisions, _ := a.store.Decisions()
	ruleList, _ := a.store.Rules()
	for _, s := range rules.SuggestRules(decisions, ruleList, a.store.Ignored) {
		if s.Key != in.Key {
			continue
		}
		rule := s.Rule()
		if err := a.store.PutRule(&rule); err != nil {
			fail(w, err)
			return
		}
		ids := []int64{rule.ID}
		for _, old := range ruleList {
			ids = append(ids, old.ID)
		}
		a.store.OrderRules(ids)
		a.events.publish("rules")
		writeJSON(w, http.StatusOK, rule)
		return
	}
	writeJSON(w, http.StatusNotFound, errBody("Vorschlag nicht mehr aktuell"))
}

// --- settings ---

type dirView struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Replays int    `json:"replays"`
}

func countReplays(dir string) (int, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".replay") && !strings.HasPrefix(e.Name(), "bc-") {
			n++
		}
	}
	return n, true
}

func (a *App) dirViews() []dirView {
	out := []dirView{}
	for _, d := range a.demoDirs() {
		n, exists := countReplays(d)
		out = append(out, dirView{Path: d, Exists: exists, Replays: n})
	}
	return out
}

type settingsView struct {
	config.Config
	Account   accountView      `json:"account"`
	Autostart bool             `json:"autostart"`
	Dirs      []dirView        `json:"dirs"`
	Defaults  []string         `json:"default_dirs"`
	Players   []store.Identity `json:"players"`
	LogPath   string           `json:"log_path"`
}

func (a *App) settingsView() settingsView {
	cfg := a.config()
	cfg.Token, cfg.TokenProtected = "", ""
	ids, _ := a.store.Identities()
	if ids == nil {
		ids = []store.Identity{}
	}
	defaults, _ := game.DefaultDemoDirs()
	return settingsView{Config: cfg, Account: a.accountInfo(), Autostart: a.desk.Autostart(), Dirs: a.dirViews(),
		Defaults: defaults, Players: ids, LogPath: a.opts.LogPath}
}

func (a *App) apiSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.settingsView())
}

func (a *App) apiUpdateSettings(w http.ResponseWriter, r *http.Request) {
	patch, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := a.UpdateSettings(patch); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, a.settingsView())
}

func (a *App) apiSetToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	me, err := a.SetToken(r.Context(), strings.TrimSpace(in.Token))
	if errors.Is(err, ballchasing.ErrBadToken) {
		writeJSON(w, http.StatusBadRequest, errBody("ballchasing kennt diesen Token nicht"))
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, me)
}

func (a *App) apiRemoveToken(w http.ResponseWriter, r *http.Request) {
	if err := a.RemoveToken(); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiAutostart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		On bool `json:"on"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := a.desk.SetAutostart(in.On); err != nil {
		fail(w, err)
		return
	}
	a.events.publish("settings")
	ok(w)
}

// apiTestKey waits a few seconds for the user to switch to the game, then
// holds the save key once.
func (a *App) apiTestKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key   string `json:"key"`
		Delay int    `json:"delay"`
	}
	if !decode(w, r, &in) {
		return
	}
	cfg := a.config()
	if in.Key == "" {
		in.Key = cfg.MarkKey
	}
	if !game.ValidKey(in.Key) {
		writeJSON(w, http.StatusBadRequest, errBody("Diese Taste wird nicht unterstützt"))
		return
	}
	select {
	case <-r.Context().Done():
		return
	case <-time.After(time.Duration(min(max(in.Delay, 0), 15)) * time.Second):
	}
	err := game.PressSaveKey(r.Context(), in.Key, time.Duration(cfg.MarkHoldMs)*time.Millisecond)
	if err != nil {
		if errors.Is(err, game.ErrNotFocused) {
			err = errors.New("Rocket League war nicht im Vordergrund, nichts gedrückt")
		}
		writeJSON(w, http.StatusOK, errBody(err.Error()))
		return
	}
	ok(w)
}

func (a *App) apiIdentities(w http.ResponseWriter, r *http.Request) {
	cur, _ := a.store.Identities()
	if cur == nil {
		cur = []store.Identity{}
	}
	cands := a.identityCandidates()
	if cands == nil {
		cands = []store.Identity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "candidates": cands, "players": a.knownPlayers()})
}

// knownPlayers lists every player of the user's own uploads, for picking
// accounts by hand.
func (a *App) knownPlayers() []mateView {
	count := map[string]int{}
	names := a.playerNames()
	for _, r := range a.visible() {
		if r.Own {
			for _, p := range r.Players {
				if p.PID != "" {
					count[p.PID]++
				}
			}
		}
	}
	out := []mateView{}
	for pid, n := range count {
		out = append(out, mateView{PID: pid, Name: firstNonEmpty(names[pid], pid), Games: n})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Games > out[j].Games || out[i].Games == out[j].Games && out[i].Name < out[j].Name
	})
	return out[:min(len(out), 30)]
}

func (a *App) apiSetIdentities(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PIDs []string `json:"pids"`
	}
	if !decode(w, r, &in) {
		return
	}
	names := a.playerNames()
	var ids []store.Identity
	for _, pid := range in.PIDs {
		if strings.Contains(pid, ":") {
			ids = append(ids, store.Identity{PID: pid, Name: names[pid], Confirmed: true})
		}
	}
	if err := a.store.SetIdentities(ids); err != nil {
		fail(w, err)
		return
	}
	a.dedupe()
	a.events.publish("state")
	a.events.publish("replays")
	a.requestSync(false) // games with the new accounts
	ok(w)
}

// --- first-run setup ---

func (a *App) apiSetup(w http.ResponseWriter, r *http.Request) {
	month := a.now().AddDate(0, 0, -30)
	groups, _ := a.store.Groups()
	if groups == nil {
		groups = []store.Group{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account":  a.accountInfo(),
		"dirs":     a.dirViews(),
		"old":      map[string]int{"last30": a.queue.CountSeeded(month), "all": a.queue.CountSeeded(time.Time{})},
		"defaults": rules.Defaults(),
		"groups":   groups,
		"syncing":  a.Syncing(),
	})
}

// apiSetupRules replaces the rules with the ones picked in the setup.
func (a *App) apiSetupRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Rules []store.Rule `json:"rules"`
	}
	if !decode(w, r, &in) {
		return
	}
	old, _ := a.store.Rules()
	for _, rule := range old {
		a.store.DeleteRule(rule.ID)
	}
	for i := range in.Rules {
		rule := in.Rules[i]
		rule.ID, rule.Pos = 0, i+1
		if err := a.store.PutRule(&rule); err != nil {
			fail(w, err)
			return
		}
	}
	a.events.publish("rules")
	ok(w)
}

func (a *App) apiBackfill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"` // none, 30, all
	}
	if !decode(w, r, &in) {
		return
	}
	var since time.Time
	switch in.Mode {
	case "none":
		ok(w)
		return
	case "30":
		since = a.now().AddDate(0, 0, -30)
	}
	n, err := a.queue.QueueSeeded(since)
	if err != nil {
		fail(w, err)
		return
	}
	log.Printf("%d ältere Replays werden nachträglich hochgeladen", n)
	writeJSON(w, http.StatusOK, map[string]int{"queued": n})
}

func (a *App) apiSetupDone(w http.ResponseWriter, r *http.Request) {
	if _, err := a.UpdateSettings([]byte(`{"setup_done":true}`)); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}
