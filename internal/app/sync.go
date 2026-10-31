package app

import (
	"context"
	"errors"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/ballchasing"
	"github.com/EYGjamie/roMod-Rocket-League/internal/store"
)

// syncKind says how much a sync reads.
type syncKind int

const (
	syncNew      syncKind = iota // new uploads only
	syncFull                     // all own replays, which drops replays deleted on the website, and the folder tree
	syncComplete                 // like syncFull, looking into every folder for subfolders
)

const (
	syncIdleEvery = time.Hour      // new uploads, unless played lately
	fullSyncEvery = 24 * time.Hour // all own replays, if the last full sync is older
)

// Variables for tests.
var (
	syncEvery = 15 * time.Minute // new uploads after playing lately
	idleCheck = 5 * time.Second  // how often held work looks whether the game closed
)

// requestSync asks the sync loop for a run of at least the given kind.
func (a *App) requestSync(kind syncKind) {
	select {
	case a.syncWake <- kind:
		return
	default:
	}
	select { // replace a pending request that reads less
	case old := <-a.syncWake:
		kind = max(kind, old)
	default:
	}
	select {
	case a.syncWake <- kind:
	default:
	}
}

// syncLoop syncs at start, then looks for new uploads every 15 minutes
// after playing lately, otherwise hourly; fully if the last full sync is a
// day old. These automatic runs wait while Rocket League runs and start
// once it closed; runs the interface asks for start right away.
func (a *App) syncLoop(ctx context.Context) {
	kind, auto, due := syncNew, true, true
	t := time.NewTicker(syncEvery)
	defer t.Stop()
	held := time.NewTicker(idleCheck)
	defer held.Stop()
	var last time.Time
	pinged := false
	for {
		switch {
		case !due || a.currentClient() == nil:
		case !auto || !a.gameOpen():
			if auto && a.fullSyncDue() {
				kind = max(kind, syncFull)
			}
			a.syncAll(ctx, kind)
			last, kind, auto, due = a.now(), syncNew, true, false
		case a.myAccount().SteamID == "" && !pinged:
			a.ping(ctx) // e.g. started while the game runs: uploads need the account
			pinged = true
		}
		select {
		case <-ctx.Done():
			return
		case k := <-a.syncWake:
			kind, auto, due = max(kind, k), false, true
		case <-t.C:
			due = due || a.playing() || a.now().Sub(last) >= syncIdleEvery
			pinged = false
		case <-held.C: // a held run may start now
		}
	}
}

func (a *App) fullSyncDue() bool {
	t, err := time.Parse(time.RFC3339, a.store.Meta("full_synced"))
	return err != nil || a.now().Sub(t) >= fullSyncEvery
}

// playing reports whether Rocket League runs or a game ended within the
// last hour: then teammates upload games with the user in them.
func (a *App) playing() bool {
	a.mu.RLock()
	last := a.lastGame
	a.mu.RUnlock()
	return a.statsAPI.Load() || a.now().Sub(last) < time.Hour
}

// gameOpen reports whether Rocket League runs, as far as the Stats API
// shows. Work in the background waits meanwhile, so only what the matches
// need goes over the network while the user plays: uploads between
// matches and the rank lookup.
func (a *App) gameOpen() bool { return a.statsAPI.Load() || a.tracker.InMatch() }

// waitGame returns once Rocket League is closed; false if ctx ended first.
func (a *App) waitGame(ctx context.Context) bool {
	for a.gameOpen() {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(idleCheck):
		}
	}
	return ctx.Err() == nil
}

// Syncing reports whether a sync runs.
func (a *App) Syncing() bool { return a.syncing.Load() }

func (a *App) syncAll(ctx context.Context, kind syncKind) {
	if !a.syncing.CompareAndSwap(false, true) {
		return
	}
	defer a.syncing.Store(false)
	a.events.publish("state")
	defer a.events.publish("state")

	a.ping(ctx)
	if a.myAccount().SteamID == "" {
		return
	}
	full := kind >= syncFull
	if full || a.store.Meta("groups_synced") == "" {
		if err := a.syncGroups(ctx, kind == syncComplete); err != nil {
			log.Printf("Ordner abgleichen: %v", err)
		}
	}
	if err := a.syncOwn(ctx, full); err != nil {
		log.Printf("Eigene Replays abgleichen: %v", err)
	} else if full {
		a.store.SetMeta("full_synced", a.now().Format(time.RFC3339))
	}
	a.ensureIdentities()
	if err := a.syncMates(ctx); err != nil {
		log.Printf("Spiele mit dir abgleichen: %v", err)
	}
	a.dedupe()
	a.events.publish("replays")
	a.events.publish("groups")
	a.events.publish("inbox")
}

// syncGroups reads the whole group tree. ballchasing lists only top-level
// groups and the children of one group at a time, so this takes one call
// per group that can have subgroups. Unless complete, a known group without
// known subgroups and without replays in subgroups counts as a leaf: daily
// folders would otherwise add one call per day played.
func (a *App) syncGroups(ctx context.Context, complete bool) error {
	c := a.currentClient()
	if c == nil {
		return nil
	}
	old, _ := a.store.Groups()
	known, hasSub := map[string]bool{}, map[string]bool{}
	for _, g := range old {
		known[g.ID], hasSub[g.Parent] = true, true
	}
	var all []store.Group
	parents := []string{""}
	for len(parents) > 0 && len(all) < 2000 {
		parent := parents[0]
		parents = parents[1:]
		list, err := c.ListGroups(ctx, parent)
		if err != nil {
			return err
		}
		for _, g := range list {
			created, _ := time.Parse(time.RFC3339, g.Created)
			all = append(all, store.Group{ID: g.ID, Name: g.Name, Parent: parent, Created: created,
				Direct: g.DirectReplays, Indirect: g.IndirectReplays})
			if complete || !known[g.ID] || hasSub[g.ID] || g.IndirectReplays > g.DirectReplays {
				parents = append(parents, g.ID)
			}
		}
	}
	if err := a.store.ReplaceGroups(all); err != nil {
		return err
	}
	a.store.SetMeta("groups_synced", a.now().Format(time.RFC3339))
	a.events.publish("groups")
	return nil
}

// syncOwn reads the account's replays: all of them when full (which also
// drops replays deleted on the website), otherwise only new uploads.
func (a *App) syncOwn(ctx context.Context, full bool) error {
	c := a.currentClient()
	me := a.myAccount()
	q := ballchasing.ReplayQuery{Uploader: "me", Oldest: true}
	if cursor, err := time.Parse(time.RFC3339, a.store.Meta("own_cursor")); err == nil && !full {
		q.CreatedAfter = cursor
	}
	seen := map[string]bool{}
	latest := q.CreatedAfter
	next := ""
	for {
		page, err := c.ListReplays(ctx, q, next)
		if err != nil {
			return err
		}
		for _, r := range page.List {
			rep := store.FromAPI(r, me.SteamID)
			rep.Own = true
			if err := a.store.PutReplay(rep); err != nil {
				return err
			}
			seen[rep.ID] = true
			if rep.Created.After(latest) {
				latest = rep.Created
			}
		}
		if page.Next == "" {
			break
		}
		next = page.Next
	}
	if !latest.IsZero() {
		a.store.SetMeta("own_cursor", latest.Format(time.RFC3339))
	}
	if full {
		replays, _ := a.store.Replays()
		for _, r := range replays {
			if r.Own && !seen[r.ID] {
				a.store.DeleteReplay(r.ID)
			}
		}
	}
	return nil
}

// syncMates reads replays other people uploaded with one of the user's
// accounts in them. Private ones are not visible to us.
func (a *App) syncMates(ctx context.Context) error {
	c := a.currentClient()
	me := a.myAccount()
	ids, _ := a.store.Identities()
	for _, id := range ids {
		key := "mates_cursor:" + id.PID
		q := ballchasing.ReplayQuery{PlayerID: id.PID, Oldest: true}
		if cursor, err := time.Parse(time.RFC3339, a.store.Meta(key)); err == nil {
			q.CreatedAfter = cursor
		}
		latest := q.CreatedAfter
		next := ""
		for {
			page, err := c.ListReplays(ctx, q, next)
			if err != nil {
				return err
			}
			for _, r := range page.List {
				if r.Uploader.SteamID == me.SteamID {
					continue
				}
				rep := store.FromAPI(r, me.SteamID)
				if err := a.store.PutReplay(rep); err != nil {
					return err
				}
				if rep.Created.After(latest) {
					latest = rep.Created
				}
			}
			if page.Next == "" {
				break
			}
			next = page.Next
		}
		if !latest.IsZero() {
			a.store.SetMeta(key, latest.Format(time.RFC3339))
		}
	}
	return nil
}

const (
	detailEvery   = 6 * time.Second
	detailOffline = time.Minute // pause while ballchasing is unreachable or the token invalid
)

// detailRetry is when to ask again for a replay that brought no statistics.
type detailRetry struct {
	at   time.Time
	wait time.Duration
}

// detailLoop fetches missing statistics slowly in the background, keeping
// well below the hourly limit of the replay endpoint. It rests while
// Rocket League runs and while ballchasing is unreachable. A replay that brings no
// statistics (still processing, failed, an error) is asked for again after
// 15 minutes, then less and less often, instead of every few seconds.
func (a *App) detailLoop(ctx context.Context) {
	later := map[string]detailRetry{}
	wait := detailEvery
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = a.detailStep(ctx, later)
	}
}

// detailStep fetches the statistics of the next due replay, if any, and
// returns the pause before the next step.
func (a *App) detailStep(ctx context.Context, later map[string]detailRetry) time.Duration {
	if watch, _ := a.pausedState(); watch || a.gameOpen() || a.currentClient() == nil || a.queueBusy() {
		return detailEvery
	}
	id := a.nextWithoutDetail(later)
	if id == "" {
		return detailEvery
	}
	err := a.fetchDetail(ctx, id)
	switch {
	case err != nil && (ballchasing.Unavailable(err) || errors.Is(err, ballchasing.ErrBadToken)):
		return detailOffline
	case a.hasDetail(id):
		delete(later, id)
	default:
		r := later[id]
		r.wait = min(max(2*r.wait, 15*time.Minute), 24*time.Hour)
		r.at = a.now().Add(r.wait)
		later[id] = r
	}
	return detailEvery
}

// nextWithoutDetail returns the next replay to fetch statistics for, "" if
// none is due.
func (a *App) nextWithoutDetail(later map[string]detailRetry) string {
	ids, _ := a.store.WithoutDetail(len(later) + 1)
	now := a.now()
	for _, id := range ids {
		if r, ok := later[id]; !ok || !now.Before(r.at) {
			return id
		}
	}
	return ""
}

// hasDetail reports whether a replay's statistics are stored, or the
// replay is gone.
func (a *App) hasDetail(id string) bool {
	r, ok, _ := a.store.Replay(id)
	return !ok || r.HasDetail
}

func (a *App) queueBusy() bool {
	files, _ := a.store.Files(store.Uploading, store.Processing)
	return len(files) > 0
}

// fetchDetail loads and stores the statistics of one replay.
func (a *App) fetchDetail(ctx context.Context, id string) error {
	c := a.currentClient()
	if c == nil {
		return nil
	}
	r, raw, err := c.GetReplay(ctx, id)
	if err != nil {
		if ballchasing.HasStatus(err, 404) {
			a.store.DeleteReplay(id) // gone on ballchasing
		}
		return err
	}
	if r.Status != "ok" {
		return nil
	}
	old, _, _ := a.store.Replay(id)
	rep := store.FromAPI(r, a.myAccount().SteamID)
	rep.Own = old.Own || rep.Own
	if err := a.store.PutDetail(rep, raw); err != nil {
		return err
	}
	a.dedupeAfter(old, id)
	a.events.publish("replays")
	return nil
}

// ensureIdentities guesses the user's player accounts from their uploads
// if none are known yet: the most frequent player, plus a second account
// that is frequent too but never in the same game.
func (a *App) ensureIdentities() {
	if ids, _ := a.store.Identities(); len(ids) > 0 {
		return
	}
	cands := a.identityCandidates()
	if len(cands) == 0 {
		return
	}
	a.store.SetIdentities(cands)
	a.events.publish("state")
}

// identityCandidates proposes the user's accounts.
func (a *App) identityCandidates() []store.Identity {
	replays, _ := a.store.Replays()
	count := map[string]int{}
	names := map[string]string{}
	together := map[[2]string]bool{}
	own := 0
	for _, r := range replays {
		if !r.Own {
			continue
		}
		own++
		var pids []string
		for _, p := range r.Players {
			if p.PID == "" {
				continue
			}
			count[p.PID]++
			if names[p.PID] == "" {
				names[p.PID] = p.Name
			}
			pids = append(pids, p.PID)
		}
		for _, x := range pids {
			for _, y := range pids {
				together[[2]string{x, y}] = true
			}
		}
	}
	type cand struct {
		pid string
		n   int
	}
	var list []cand
	for pid, n := range count {
		list = append(list, cand{pid, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].pid < list[j].pid
	})
	if len(list) == 0 || own == 0 {
		return nil
	}
	out := []store.Identity{{PID: list[0].pid, Name: names[list[0].pid]}}
	for _, c := range list[1:] {
		if c.n*10 >= own*3 && !together[[2]string{list[0].pid, c.pid}] {
			out = append(out, store.Identity{PID: c.pid, Name: names[c.pid]})
			break
		}
	}
	return out
}

// dedupe hides further uploads of the same game behind one entry, the own
// upload if there is one. Uploads with known match GUIDs are compared by
// those, the others by map, score, players and a date within ten minutes.
func (a *App) dedupe() {
	replays, err := a.store.Replays()
	if err != nil {
		return
	}
	byKey := map[string][]store.Replay{}
	for _, r := range replays {
		k := gameKey(r)
		byKey[k] = append(byKey[k], r)
	}
	for _, list := range byKey {
		a.dedupeBucket(list)
	}
}

// dedupeAfter does the same as dedupe after replay id was stored, but only
// for the uploads that can be the same game. old is the replay as it was
// before (the zero Replay if new), as its key may have changed.
func (a *App) dedupeAfter(old store.Replay, id string) {
	cur, ok, err := a.store.Replay(id)
	if err != nil {
		return
	}
	check := map[string]store.Replay{}
	if old.ID != "" {
		check[gameKey(old)] = old
	}
	if ok {
		check[gameKey(cur)] = cur
	}
	for k, r := range check {
		same, err := a.store.ReplaysWithScore(r.MapCode, r.BlueGoals, r.OrangeGoals)
		if err != nil {
			return
		}
		var list []store.Replay
		for _, s := range same {
			if gameKey(s) == k {
				list = append(list, s)
			}
		}
		a.dedupeBucket(list)
	}
}

// gameKey is equal for uploads that can be the same game: same map, score
// and players.
func gameKey(r store.Replay) string {
	var pids []string
	for _, p := range r.Players {
		pids = append(pids, p.PID)
	}
	sort.Strings(pids)
	return strings.Join([]string{r.MapCode, strconv.Itoa(r.BlueGoals), strconv.Itoa(r.OrangeGoals), strings.Join(pids, ",")}, "|")
}

// dedupeBucket marks the duplicates among uploads with the same gameKey.
func (a *App) dedupeBucket(list []store.Replay) {
	if len(list) < 2 {
		for _, r := range list {
			if r.DupOf != "" {
				a.store.SetDupOf(r.ID, "")
			}
		}
		return
	}
	// preferred first: own uploads, then the earliest upload
	slices.SortFunc(list, func(x, y store.Replay) int {
		if x.Own != y.Own {
			if x.Own {
				return -1
			}
			return 1
		}
		return x.Created.Compare(y.Created)
	})
	var keep []store.Replay
	for _, r := range list {
		dupOf := ""
		for _, k := range keep {
			if sameGame(k, r) {
				dupOf = k.ID
				break
			}
		}
		if dupOf == "" {
			keep = append(keep, r)
		}
		if r.DupOf != dupOf {
			a.store.SetDupOf(r.ID, dupOf)
		}
	}
}

func sameGame(a, b store.Replay) bool {
	if a.MatchGUID != "" && b.MatchGUID != "" {
		return a.MatchGUID == b.MatchGUID
	}
	d := a.Date.Sub(b.Date)
	return d < 10*time.Minute && d > -10*time.Minute
}
