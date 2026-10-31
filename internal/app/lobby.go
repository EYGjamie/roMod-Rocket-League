package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
	"github.com/EYGjamie/roMod-Rocket-League/internal/hotkey"
	"github.com/EYGjamie/roMod-Rocket-League/internal/store"
)

// The developer mode's lobby lookup: at the start of an online match the
// Stats API lists the players, a second Epic account looks up their ranks
// and wins, and a popup at the top shows them like the scoreboard of the
// game. Hotkeys mark a smurf in the lobby and switch the playlist whose
// rank it shows. Every lobby is kept for the analysis.

// LobbyPopup is the popup with the ranks of the lobby.
type LobbyPopup struct {
	Title   string // e.g. "Lobby · 2v2"
	Note    string // state of the lookup, "" when done
	Busy    bool   // a lookup runs
	Smurf   bool
	Size    int       // players per team
	Rank    int       // the playlist whose rank is shown, 1 to 3
	Avg     [2]string // average MMR of the teams in that rank, "–" if none
	Hints   []LobbyHint
	Seconds int // until the popup closes
	Teams   [2][]LobbyRow
}

// LobbyHint is a hotkey of the popup, e.g. Key "Strg+Alt+S", Action "Smurf".
type LobbyHint struct{ Key, Action string }

// LobbyRow is one player of the popup.
type LobbyRow struct {
	Name  string
	Me    bool
	Ranks [3]LobbyRank // 1s, 2s, 3s
	Wins  string       // "" if not known
}

// LobbyRank is a rank as shown, e.g. Label "C2" and MMR "1123".
type LobbyRank struct {
	Label string
	MMR   string
	Tier  int // 0 unranked or not known
}

// lobbySettle gathers players who show up together into one lookup. The
// lookup does not wait for the lobby to fill: the Stats API sends the
// player list once a second as set up by the tool, and players who load
// later get a lookup of their own that adds them to the popup.
var lobbySettle = 200 * time.Millisecond

type lobbyTracker struct {
	a *App

	mu      sync.Mutex
	cur     *store.Lobby // the latest online match
	shown   bool         // shown on its own for cur
	visible bool
	seq     int // of the popup, so an old timer does not close a new one
	hideT   *time.Timer
	lookT   *time.Timer
	busy    bool   // a lookup runs
	err     string // of the last lookup for cur
	rank    int    // playlist picked with the hotkey since the start, 0 none
}

// validPlayer reports whether a Stats API ID belongs to a real account, not
// a bot.
func validPlayer(id string) bool {
	parts := strings.Split(id, "|")
	return len(parts) == 3 && parts[0] != "" && parts[0] != "Unknown" && parts[1] != "" && parts[1] != "0"
}

// ballchasingPID turns Steam|7656…|0 into steam:7656…, as ballchasing and
// the user's accounts write it.
func ballchasingPID(id string) string {
	parts := strings.Split(id, "|")
	if len(parts) < 2 {
		return ""
	}
	platform := strings.ToLower(parts[0])
	if platform == "xboxone" {
		platform = "xbox"
	}
	return platform + ":" + parts[1]
}

// players takes the player list of an UpdateState; it runs often, so it
// only does work when the list changed.
func (l *lobbyTracker) players(guid string, ps []game.Player) {
	if !l.a.config().LobbyOn() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur == nil || l.cur.GUID != guid {
		l.start(guid)
	}
	present := map[string]bool{}
	joined, changed := false, false
	for _, p := range ps {
		if !validPlayer(p.PrimaryID) {
			continue
		}
		present[p.PrimaryID] = true
		i := slices.IndexFunc(l.cur.Players, func(lp store.LobbyPlayer) bool { return lp.ID == p.PrimaryID })
		if i < 0 {
			mine := l.a.myIDs()[ballchasingPID(p.PrimaryID)]
			l.cur.Players = append(l.cur.Players, store.LobbyPlayer{ID: p.PrimaryID, Name: p.Name, Team: p.Team, Me: mine, Wins: -1})
			joined, changed = true, true
			continue
		}
		lp := &l.cur.Players[i]
		if lp.Left || lp.Team != p.Team || lp.Name != p.Name {
			lp.Left, lp.Team, lp.Name = false, p.Team, p.Name
			changed = true
		}
	}
	if len(present) == 0 {
		return // an empty list while the match loads
	}
	var perTeam [2]int
	for i := range l.cur.Players {
		lp := &l.cur.Players[i]
		if !present[lp.ID] && !lp.Left {
			lp.Left, changed = true, true
		}
		if !lp.Left && lp.Team >= 0 && lp.Team < 2 {
			perTeam[lp.Team]++
		}
	}
	if size := max(perTeam[0], perTeam[1]); size > l.cur.Size {
		l.cur.Size, changed = size, true
	}
	if !changed {
		return
	}
	l.save()
	if joined {
		if l.lookT != nil {
			l.lookT.Stop()
		}
		cur := l.cur
		l.lookT = time.AfterFunc(lobbySettle, func() { l.lookup(cur) })
	}
	if l.visible {
		l.show()
	}
}

// start begins a new lobby, or takes up a stored one, e.g. after the game
// reconnected to the match. l.mu is held.
func (l *lobbyTracker) start(guid string) {
	if l.lookT != nil {
		l.lookT.Stop()
	}
	l.shown, l.err = false, ""
	if old, ok, _ := l.a.store.Lobby(guid); ok {
		l.cur, l.shown = &old, true
		return
	}
	l.cur = &store.Lobby{GUID: guid, Started: l.a.now(), Winner: -1}
	log.Printf("Lobby erkannt (%s)", guid)
}

// save stores cur; l.mu is held.
func (l *lobbyTracker) save() {
	if err := l.a.store.PutLobby(*l.cur); err != nil {
		log.Printf("Lobby speichern: %v", err)
	}
	l.a.events.publish("lobby")
}

// lookup asks for ranks and wins of the players of lob not yet looked up,
// then shows the popup once per match.
func (l *lobbyTracker) lookup(lob *store.Lobby) {
	signedIn := l.a.mmr.SignedIn() // never asked under l.mu: the Stats API waits for it
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return // the running lookup starts another one for the rest
	}
	var ids []string
	for _, p := range lob.Players {
		if !p.Looked {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 || !signedIn {
		if !signedIn && lob == l.cur {
			l.err = "Nicht bei Epic angemeldet"
		}
		l.mu.Unlock()
		return
	}
	l.busy = true
	if l.visible && lob == l.cur {
		l.show()
	}
	l.mu.Unlock()

	ctx, cancel := context.WithTimeout(l.a.ctx, 30*time.Second)
	found, err := l.a.mmr.Lookup(ctx, ids)
	cancel()
	l.a.epicResult(err)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.busy = false
	if err == nil {
		for i := range lob.Players {
			p := &lob.Players[i]
			if !slices.Contains(ids, p.ID) {
				continue
			}
			p.Looked = true
			if f, ok := found[p.ID]; ok {
				p.Wins = f.Wins
				for j, s := range f.Ranks {
					if s != nil {
						p.Ranks[j] = &store.Rank{Tier: s.Tier, Div: s.Div, MMR: s.MMR, Played: s.Played}
					}
				}
			}
		}
		log.Printf("MMR-Abfrage: %d Spieler nachgeschlagen, %d gefunden", len(ids), len(found))
	}
	if lob != l.cur {
		if err := l.a.store.PutLobby(*lob); err != nil { // the next match began meanwhile
			log.Printf("Lobby speichern: %v", err)
		}
		return
	}
	l.err = ""
	if err != nil {
		l.err = err.Error()
	}
	l.save()
	if err == nil && slices.ContainsFunc(lob.Players, func(p store.LobbyPlayer) bool { return !p.Looked }) {
		go l.lookup(lob) // joined during the lookup
	}
	switch {
	case !l.shown && l.a.config().LobbyPopup:
		l.shown = true
		l.show()
	case l.visible:
		l.show()
	}
}

// winner records who won the match.
func (l *lobbyTracker) winner(guid string, team int) {
	if !l.a.config().LobbyOn() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur != nil && l.cur.GUID == guid {
		l.cur.Winner = team
		l.save()
	}
}

// view builds the popup; l.mu is held.
func (l *lobbyTracker) view() LobbyPopup {
	cfg := l.a.config()
	v := LobbyPopup{Title: "Lobby", Avg: [2]string{"–", "–"}, Seconds: cfg.LobbySeconds}
	if l.a.opts.Hotkeys != nil {
		if cfg.SmurfHotkey != "" {
			v.Hints = append(v.Hints, LobbyHint{hotkey.Label(cfg.SmurfHotkey), "Smurf"})
		}
		if cfg.RankHotkey != "" && l.a.desk.Platform().Scoreboard {
			v.Hints = append(v.Hints, LobbyHint{hotkey.Label(cfg.RankHotkey), "1s/2s/3s"})
		}
		if cfg.LobbyHotkey != "" {
			v.Hints = append(v.Hints, LobbyHint{hotkey.Label(cfg.LobbyHotkey), "schließen"})
		}
	}
	v.Rank = l.playlist(cfg)
	lob := l.cur
	if lob == nil {
		v.Note = "Noch kein Online-Match seit dem Start"
		return v
	}
	if lob.Size > 0 {
		v.Title = fmt.Sprintf("Lobby · %dv%d", lob.Size, lob.Size)
	}
	v.Size, v.Smurf, v.Busy = lob.Size, lob.Smurf, l.busy
	switch {
	case l.busy:
		v.Note = "Ränge werden geladen …"
	case l.err != "":
		v.Note = l.err
	}
	cur := v.Rank - 1
	var sum [2]float64
	var n [2]int
	for _, p := range lob.Players {
		if p.Left || p.Team < 0 || p.Team > 1 {
			continue
		}
		if r := p.Ranks[cur]; r != nil {
			sum[p.Team] += r.MMR
			n[p.Team]++
		}
		row := LobbyRow{Name: p.Name, Me: p.Me}
		if p.Wins >= 0 {
			row.Wins = thousands(p.Wins)
		}
		for i, r := range p.Ranks {
			if r != nil {
				row.Ranks[i] = LobbyRank{Label: tierShort(r.Tier), MMR: strconv.Itoa(int(r.MMR + 0.5)), Tier: r.Tier}
			} else if p.Looked {
				row.Ranks[i] = LobbyRank{Label: "–"}
			}
		}
		if p.Me {
			v.Teams[p.Team] = append([]LobbyRow{row}, v.Teams[p.Team]...)
		} else {
			v.Teams[p.Team] = append(v.Teams[p.Team], row)
		}
	}
	for t := range v.Avg {
		if n[t] > 0 {
			v.Avg[t] = strconv.Itoa(int(sum[t]/float64(n[t]) + 0.5))
		}
	}
	return v
}

// show puts up the popup, or updates it, and closes it after the
// configured time; l.mu is held.
func (l *lobbyTracker) show() {
	l.a.desk.ShowLobby(l.view())
	l.visible = true
	l.seq++
	seq := l.seq
	if l.hideT != nil {
		l.hideT.Stop()
	}
	l.hideT = time.AfterFunc(time.Duration(l.a.config().LobbySeconds)*time.Second, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.seq == seq {
			l.hideLocked()
		}
	})
}

func (l *lobbyTracker) hideLocked() {
	if l.hideT != nil {
		l.hideT.Stop()
	}
	if l.visible {
		l.a.desk.HideLobby()
	}
	l.visible = false
}

func (l *lobbyTracker) hide() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hideLocked()
}

// toggle opens or closes the popup, for the hotkey. Opening it retries a
// lookup that failed.
func (l *lobbyTracker) toggle() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.visible {
		l.hideLocked()
		return
	}
	l.show()
	if lob := l.cur; lob != nil && !l.busy && slices.ContainsFunc(lob.Players, func(p store.LobbyPlayer) bool { return !p.Looked }) {
		go l.lookup(lob)
	}
}

// toggleSmurf marks or unmarks a smurf in the current lobby, for the
// hotkey, and shows the popup as confirmation.
func (l *lobbyTracker) toggleSmurf() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur != nil {
		l.cur.Smurf = !l.cur.Smurf
		l.save()
		log.Printf("Lobby %s: Smurf %s", l.cur.GUID, map[bool]string{true: "markiert", false: "nicht mehr markiert"}[l.cur.Smurf])
	}
	l.show()
}

// playlist is the playlist whose rank the popup shows, 1 to 3: the one
// picked with the hotkey, else the setting's, else the match's; l.mu is
// held.
func (l *lobbyTracker) playlist(cfg config.Config) int {
	switch {
	case l.rank > 0:
		return l.rank
	case cfg.LobbyRank > 0:
		return cfg.LobbyRank
	case l.cur != nil:
		return min(max(l.cur.Size, 1), 3)
	}
	return 1
}

// cycleRank switches the popup to the next playlist, 1s, 2s, 3s, for the
// hotkey, and shows it. The pick holds until the program ends; the next
// start begins with the setting again.
func (l *lobbyTracker) cycleRank() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rank = l.playlist(l.a.config())%3 + 1
	l.show()
}

// resetRank drops the pick of the hotkey, when the setting changes.
func (l *lobbyTracker) resetRank() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rank = 0
	if l.visible {
		l.show()
	}
}

// setSmurf marks a stored lobby, from the interface.
func (l *lobbyTracker) setSmurf(guid string, smurf bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur != nil && l.cur.GUID == guid {
		l.cur.Smurf = smurf
		l.save()
		if l.visible {
			l.show()
		}
		return nil
	}
	if err := l.a.store.SetSmurf(guid, smurf); err != nil {
		return err
	}
	l.a.events.publish("lobby")
	return nil
}

type lobbyState struct {
	Lobby *store.Lobby `json:"lobby"` // nil before the first online match
	Busy  bool         `json:"busy"`
	Error string       `json:"error,omitempty"`
}

func (l *lobbyTracker) state() lobbyState {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := lobbyState{Busy: l.busy, Error: l.err}
	if l.cur != nil {
		c := *l.cur
		c.Players = slices.Clone(c.Players)
		s.Lobby = &c
	}
	return s
}

// tierShort is a rank as trackers abbreviate it: C2 is Champion II.
func tierShort(tier int) string {
	switch {
	case tier <= 0:
		return "Unr."
	case tier >= 22:
		return "SSL"
	case tier >= 19:
		return "GC" + strconv.Itoa(tier-18)
	}
	return string("BSGPDC"[(tier-1)/3]) + strconv.Itoa((tier-1)%3+1)
}

// thousands writes n with dots, as German does: 12.345.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

func (a *App) apiLobby(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.lobby.state())
}

func (a *App) apiSmurf(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Smurf bool `json:"smurf"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := a.lobby.setSmurf(r.PathValue("guid"), in.Smurf); err != nil {
		writeJSON(w, http.StatusNotFound, errBody("Lobby nicht gefunden"))
		return
	}
	ok(w)
}
