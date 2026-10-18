package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/hotkey"
	"github.com/EYGjamie/auto-replay-upload/internal/rules"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// processed runs once ballchasing has processed an upload.
func (a *App) processed(ctx context.Context, f store.File, r ballchasing.Replay, raw []byte, duplicate bool) {
	me := a.myAccount()
	rep := store.FromAPI(r, me.SteamID)
	if !duplicate {
		rep.Own = true // a 201 is always our own upload
	}
	if rep.Own {
		rep.File = f.Name
	}
	old, _, _ := a.store.Replay(rep.ID)
	if err := a.store.PutDetail(rep, raw); err != nil {
		log.Printf("Replay speichern: %v", err)
		return
	}
	a.ensureIdentities()
	a.dedupeAfter(old, rep.ID)
	a.mu.Lock()
	a.lastGame = a.now()
	a.mu.Unlock()
	a.events.publish("replays")

	if rep.Own && a.config().DeleteUploaded && !f.Manual && rep.Status == "ok" {
		path := filepath.Join(f.Dir, f.Name)
		if err := a.desk.Recycle(path); err != nil {
			log.Printf("%s nicht gelöscht: %v", f.Name, err)
		} else {
			f.Deleted = true
			a.store.PutFile(f)
			log.Printf("%s in den Papierkorb verschoben", f.Name)
		}
	}
	if !rep.Own {
		a.notifyUploaded(rep, "")
		return
	}
	a.sortNew(ctx, rep)
}

func (a *App) uploadFailed(f store.File) {
	a.updateTray()
	if !a.config().NotifyFailed {
		return
	}
	title := "Upload fehlgeschlagen"
	if f.State == store.Rejected {
		title = "Replay abgelehnt"
	}
	t := Toast{Kind: "failed", Title: title, Body: f.Name + ": " + f.Error,
		Actions: []ToastAction{{Label: "Details", Run: func() { a.desk.ShowOverlay("uebersicht") }}},
		Disable: func() { a.UpdateSettings([]byte(`{"notify_failed":false}`)) }}
	if f.State == store.Failed {
		t.Actions = append([]ToastAction{{Label: "Jetzt erneut", Run: func() { a.queue.Retry(f.Name) }}}, t.Actions...)
	}
	a.toast(t)
}

// summary describes a replay in one line, e.g. "2s Ranked · DFH Stadium ·
// Sieg 3:1".
func (a *App) summary(r store.Replay) string {
	parts := []string{rules.PlaylistName(r.PlaylistID), r.MapName}
	score := fmt.Sprintf("%d:%d", r.BlueGoals, r.OrangeGoals)
	me := a.myIDs()
	if r.MyTeam(me) == "orange" {
		score = fmt.Sprintf("%d:%d", r.OrangeGoals, r.BlueGoals)
	}
	switch r.Result(me) {
	case "win":
		score = "Sieg " + score
	case "loss":
		score = "Niederlage " + score
	}
	return strings.Join(append(parts, score), " · ")
}

// sortNew puts a fresh own upload into the folder of the first matching
// rule, after the quick-sort countdown, or leaves it in the inbox.
func (a *App) sortNew(ctx context.Context, r store.Replay) {
	ruleList, _ := a.store.Rules()
	facts := rules.FactsOf(r, a.myIDs())
	rule, ok := rules.Match(ruleList, facts)
	if !ok {
		a.events.publish("inbox")
		a.notifyUploaded(r, "")
		return
	}
	target, err := a.ruleTarget(ctx, rule)
	if err != nil {
		log.Printf("Ordner %q anlegen: %v", rule.TargetName, err)
		a.notifyUploaded(r, "")
		return
	}
	if !a.config().QuickSort {
		a.applySort(r, target, false)
		return
	}
	alts := a.suggestions(r, 4)
	var options []store.Group
	for _, s := range alts {
		if s.GroupID != target.ID && len(options) < 3 {
			options = append(options, store.Group{ID: s.GroupID, Name: s.Name})
		}
	}
	a.quick.add(quickPrompt{replay: r, target: target, options: options})
}

// ruleTarget returns the rule's group, creating it on the first match.
func (a *App) ruleTarget(ctx context.Context, rule store.Rule) (store.Group, error) {
	groups, _ := a.store.Groups()
	for _, g := range groups {
		if g.ID == rule.TargetID {
			return g, nil
		}
	}
	for _, g := range groups { // created meanwhile, e.g. by another rule
		if rule.TargetName != "" && g.Parent == "" && strings.EqualFold(g.Name, rule.TargetName) {
			rule.TargetID = g.ID
			a.store.PutRule(&rule)
			return g, nil
		}
	}
	if rule.TargetName == "" {
		return store.Group{}, errors.New("Zielordner gibt es nicht mehr")
	}
	g, err := a.CreateGroup(ctx, rule.TargetName, "", false)
	if err != nil {
		return g, err
	}
	rule.TargetID = g.ID
	a.store.PutRule(&rule)
	a.events.publish("rules")
	return g, nil
}

// applySort moves a fresh upload and tells the user where it went.
func (a *App) applySort(r store.Replay, target store.Group, manual bool) {
	if err := a.Move(a.ctx, []string{r.ID}, target.ID, manual); err != nil {
		log.Printf("Einordnen von %s: %v", r.ID, err)
	}
	a.notifyUploaded(r, target.Name)
}

func (a *App) notifyUploaded(r store.Replay, folder string) {
	if !a.config().NotifyUploaded {
		return
	}
	body := a.summary(r)
	switch {
	case !r.Own:
		body += " (von " + r.UploaderName + ")"
	case folder != "":
		body += " → " + folder
	default:
		body += " → Eingang"
	}
	link := "https://ballchasing.com/replay/" + r.ID
	actions := []ToastAction{{Label: "Auf ballchasing öffnen", Run: func() { a.desk.OpenBrowser(link) }}}
	if r.Own {
		actions = append(actions, ToastAction{Label: "Anderer Ordner", Run: func() { a.desk.ShowOverlay("verlauf/" + r.ID) }})
	}
	a.toast(Toast{Kind: "uploaded", Title: "Replay hochgeladen", Body: body, Actions: actions,
		Disable: func() { a.UpdateSettings([]byte(`{"notify_uploaded":false}`)) }})
}

// toast shows a notification, or holds it back until the match is over.
func (a *App) toast(t Toast) {
	if a.tracker.InMatch() {
		a.mu.Lock()
		a.toasts = append(a.toasts, t)
		a.mu.Unlock()
		return
	}
	a.desk.Toast(t)
}

func (a *App) flushToasts() {
	if a.tracker.InMatch() {
		return
	}
	a.mu.Lock()
	held := a.toasts
	a.toasts = nil
	a.mu.Unlock()
	for _, t := range held {
		a.desk.Toast(t)
	}
}

// suggestions returns up to n folders for a replay.
func (a *App) suggestions(r store.Replay, n int) []rules.Suggestion {
	ruleList, _ := a.store.Rules()
	decisions, _ := a.store.Decisions()
	groups, _ := a.store.Groups()
	names := map[string]string{}
	for _, g := range groups {
		names[g.ID] = a.groupPath(groups, g.ID)
	}
	return rules.Suggest(rules.FactsOf(r, a.myIDs()), ruleList, decisions, names, a.playerNames(), n)
}

// groupPath returns "Parent › Child".
func (a *App) groupPath(groups []store.Group, id string) string {
	byID := map[string]store.Group{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	var parts []string
	for seen := 0; id != "" && seen < 10; seen++ {
		g, ok := byID[id]
		if !ok {
			break
		}
		parts = append([]string{g.Name}, parts...)
		id = g.Parent
	}
	return strings.Join(parts, " › ")
}

// playerNames maps player IDs to their latest names.
func (a *App) playerNames() map[string]string {
	replays, _ := a.store.Replays()
	names := map[string]string{}
	for i := len(replays) - 1; i >= 0; i-- { // oldest first, newest name wins
		for _, p := range replays[i].Players {
			if p.PID != "" {
				names[p.PID] = p.Name
			}
		}
	}
	return names
}

// Move puts replays into a group (or out of every group with an empty
// groupID) and records the decisions for suggestions.
func (a *App) Move(ctx context.Context, ids []string, groupID string, manual bool) error {
	c := a.currentClient()
	if c == nil {
		return errors.New("nicht mit ballchasing verbunden")
	}
	groups, _ := a.store.Groups()
	name := ""
	if groupID != "" {
		name = a.groupPath(groups, groupID)
		if name == "" {
			return errors.New("Ordner nicht gefunden")
		}
	}
	me := a.myIDs()
	var errs []string
	for _, id := range ids {
		r, ok, _ := a.store.Replay(id)
		if !ok || !r.Own {
			errs = append(errs, id+": nur eigene Uploads lassen sich einordnen")
			continue
		}
		if err := c.SetGroup(ctx, id, groupID); err != nil {
			errs = append(errs, id+": "+err.Error())
			continue
		}
		leaf := name
		if i := strings.LastIndex(name, " › "); i >= 0 {
			leaf = name[i+len(" › "):]
		}
		a.store.SetReplayGroup(id, groupID, leaf)
		a.adjustCounts(r.GroupID, groupID)
		facts := rules.FactsOf(r, me)
		a.store.PutDecision(store.Decision{ReplayID: id, PlaylistID: facts.PlaylistID, Mates: facts.Mates, GroupID: groupID, Manual: manual})
	}
	a.events.publish("replays")
	a.events.publish("groups")
	a.events.publish("inbox")
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// adjustCounts keeps the folder counts current without asking ballchasing.
func (a *App) adjustCounts(from, to string) {
	if from == to {
		return
	}
	groups, _ := a.store.Groups()
	byID := map[string]store.Group{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	walk := func(id string, delta int) {
		if g, ok := byID[id]; ok {
			g.Direct += delta
			byID[id] = g
		}
		for seen := 0; id != "" && seen < 10; seen++ {
			g, ok := byID[id]
			if !ok {
				return
			}
			g.Indirect += delta
			byID[id] = g
			id = g.Parent
		}
	}
	walk(from, -1)
	walk(to, +1)
	for _, g := range byID {
		a.store.PutGroup(g)
	}
}

// CreateGroup creates a folder on ballchasing and in the local tree.
func (a *App) CreateGroup(ctx context.Context, name, parent string, clusters bool) (store.Group, error) {
	c := a.currentClient()
	if c == nil {
		return store.Group{}, errors.New("nicht mit ballchasing verbunden")
	}
	id, err := c.CreateGroup(ctx, name, parent, clusters)
	if err != nil {
		return store.Group{}, err
	}
	g := store.Group{ID: id, Name: name, Parent: parent, Created: a.now()}
	a.store.PutGroup(g)
	log.Printf("Ordner %q angelegt: https://ballchasing.com/group/%s", name, id)
	a.events.publish("groups")
	return g, nil
}

type quickPrompt struct {
	replay  store.Replay
	target  store.Group
	options []store.Group
}

// quickSorter shows one quick-sort prompt at a time.
type quickSorter struct {
	a      *App
	mu     sync.Mutex
	queue  []quickPrompt
	active *quickPrompt
	seq    int
	timer  *time.Timer
}

func (q *quickSorter) add(p quickPrompt) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, p)
	if q.active == nil {
		q.showNext()
	}
}

// showNext must be called with q.mu held.
func (q *quickSorter) showNext() {
	if len(q.queue) == 0 {
		return
	}
	p := q.queue[0]
	q.queue = q.queue[1:]
	q.active = &p
	q.seq++
	seq := q.seq
	seconds := q.a.config().QuickSortSeconds

	view := QuickSort{Title: q.a.summary(p.replay), Target: p.target.Name, Seconds: seconds}
	for i, o := range p.options {
		view.Options[i] = o.Name
	}
	view.Options[3] = "Eingang"
	q.a.desk.ShowQuickSort(view, func(choice int) { q.pick(seq, choice) })
	if hk := q.a.opts.Hotkeys; hk != nil {
		for i := 1; i <= 4; i++ {
			choice := i
			key, _ := hotkey.Parse(fmt.Sprintf("Ctrl+Alt+%d", i))
			hk.Set(fmt.Sprintf("quick%d", i), key, func() { q.pick(seq, choice) })
		}
	}
	q.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() { q.pick(seq, 0) })
}

// pick applies a choice: 0 is the rule's folder (countdown over), 1–3 an
// alternative, 4 the inbox.
func (q *quickSorter) pick(seq, choice int) {
	q.mu.Lock()
	if q.active == nil || seq != q.seq {
		q.mu.Unlock()
		return
	}
	p := *q.active
	q.active = nil
	q.timer.Stop()
	if hk := q.a.opts.Hotkeys; hk != nil {
		for i := 1; i <= 4; i++ {
			hk.Remove(fmt.Sprintf("quick%d", i))
		}
	}
	q.a.desk.HideQuickSort()
	q.showNext()
	q.mu.Unlock()

	switch {
	case choice == 4:
		q.a.store.PutDecision(store.Decision{ReplayID: p.replay.ID, PlaylistID: p.replay.PlaylistID, Manual: true})
		q.a.events.publish("inbox")
		q.a.notifyUploaded(p.replay, "")
	case choice >= 1 && choice <= len(p.options):
		q.a.applySort(p.replay, p.options[choice-1], true)
	default:
		q.a.applySort(p.replay, p.target, choice != 0)
	}
}
