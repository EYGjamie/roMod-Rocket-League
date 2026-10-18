package app

import (
	"context"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// requestSync asks the sync loop for a run; full also re-reads all own
// replays and the group tree.
func (a *App) requestSync(full bool) {
	select {
	case a.syncWake <- full:
	default:
		if full { // replace a pending quick request
			select {
			case <-a.syncWake:
			default:
			}
			select {
			case a.syncWake <- true:
			default:
			}
		}
	}
}

// syncLoop syncs fully at start and incrementally every 15 minutes.
func (a *App) syncLoop(ctx context.Context) {
	full := true
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		if a.currentClient() != nil {
			a.syncAll(ctx, full)
		}
		select {
		case <-ctx.Done():
			return
		case full = <-a.syncWake:
		case <-t.C:
			full = false
		}
	}
}

// Syncing reports whether a sync runs.
func (a *App) Syncing() bool { return a.syncing.Load() }

func (a *App) syncAll(ctx context.Context, full bool) {
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
	if full || a.store.Meta("groups_synced") == "" {
		if err := a.syncGroups(ctx); err != nil {
			log.Printf("Ordner abgleichen: %v", err)
		}
	}
	if err := a.syncOwn(ctx, full); err != nil {
		log.Printf("Eigene Replays abgleichen: %v", err)
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
// per group.
func (a *App) syncGroups(ctx context.Context) error {
	c := a.currentClient()
	if c == nil {
		return nil
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
			parents = append(parents, g.ID)
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

// detailLoop fetches missing statistics slowly in the background, keeping
// well below the hourly limit of the replay endpoint.
func (a *App) detailLoop(ctx context.Context) {
	t := time.NewTicker(6 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if watch, _ := a.pausedState(); watch || a.queueBusy() {
			continue
		}
		ids, _ := a.store.WithoutDetail(1)
		if len(ids) == 0 {
			continue
		}
		if err := a.fetchDetail(ctx, ids[0]); err != nil && ballchasing.Unavailable(err) {
			continue
		}
	}
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
	a.dedupe()
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
	type bucket struct{ list []store.Replay }
	byKey := map[string]*bucket{}
	for _, r := range replays {
		var pids []string
		for _, p := range r.Players {
			pids = append(pids, p.PID)
		}
		sort.Strings(pids)
		key := strings.Join([]string{r.MapCode, strconv.Itoa(r.BlueGoals), strconv.Itoa(r.OrangeGoals), strings.Join(pids, ",")}, "|")
		b := byKey[key]
		if b == nil {
			b = &bucket{}
			byKey[key] = b
		}
		b.list = append(b.list, r)
	}
	for _, b := range byKey {
		if len(b.list) < 2 {
			for _, r := range b.list {
				if r.DupOf != "" {
					a.store.SetDupOf(r.ID, "")
				}
			}
			continue
		}
		// preferred first: own uploads, then the earliest upload
		slices.SortFunc(b.list, func(x, y store.Replay) int {
			if x.Own != y.Own {
				if x.Own {
					return -1
				}
				return 1
			}
			return x.Created.Compare(y.Created)
		})
		var keep []store.Replay
		for _, r := range b.list {
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
}

func sameGame(a, b store.Replay) bool {
	if a.MatchGUID != "" && b.MatchGUID != "" {
		return a.MatchGUID == b.MatchGUID
	}
	d := a.Date.Sub(b.Date)
	return d < 10*time.Minute && d > -10*time.Minute
}
