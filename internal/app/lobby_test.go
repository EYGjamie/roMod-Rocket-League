package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/config"
	"github.com/EYGjamie/roMod-Rocket-League/internal/game"
	"github.com/EYGjamie/roMod-Rocket-League/internal/mmr"
)

type fakeMMR struct {
	mu       sync.Mutex
	signedIn bool
	found    map[string]mmr.Player
	err      error
	looked   [][]string
	save     func(token, name string)
}

func (f *fakeMMR) Login(ctx context.Context, code string) (string, error) {
	f.mu.Lock()
	f.signedIn = true
	f.mu.Unlock()
	f.save("refresh-"+code, "Zweitkonto")
	return "Zweitkonto", nil
}
func (f *fakeMMR) Logout()                           { f.mu.Lock(); f.signedIn = false; f.mu.Unlock(); f.save("", "") }
func (f *fakeMMR) SignedIn() bool                    { f.mu.Lock(); defer f.mu.Unlock(); return f.signedIn }
func (f *fakeMMR) Connect(ctx context.Context) error { return nil }
func (f *fakeMMR) Version() mmr.Version              { return mmr.Builtin() }
func (f *fakeMMR) Close()                            {}
func (f *fakeMMR) Lookup(ctx context.Context, ids []string) (map[string]mmr.Player, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.looked = append(f.looked, slices.Clone(ids))
	return f.found, f.err
}

func rank(tier int, mmrValue float64) *mmr.Skill { return &mmr.Skill{Tier: tier, MMR: mmrValue} }

// lobbyApp is a test app with the lobby lookup on and an Epic account.
func lobbyApp(t *testing.T) (*testApp, string) {
	t.Helper()
	old := lobbySettle
	lobbySettle = 10 * time.Millisecond
	t.Cleanup(func() { lobbySettle = old })
	ta := newTestApp(t, func(c *config.Config) { c.DevMode, c.MMR = true, true })
	me := "Steam|" + ta.fake.Me.SteamID + "|0"
	ta.mmr.signedIn = true
	ta.mmr.found = map[string]mmr.Player{
		me:          {Ranks: [3]*mmr.Skill{rank(13, 905), rank(17, 1123), nil}, Wins: 1234},
		"Epic|m|0":  {Ranks: [3]*mmr.Skill{nil, rank(16, 1050), nil}, Wins: 800},
		"Epic|o1|0": {Ranks: [3]*mmr.Skill{nil, rank(18, 1190), nil}, Wins: 5000},
		"PS4|o2|0":  {Ranks: [3]*mmr.Skill{nil, rank(19, 1260), nil}, Wins: -1},
	}
	return ta, me
}

func (ta *testApp) updateState(guid string, players ...game.Player) {
	ta.tracker.Handle(game.Event{Name: "UpdateState", MatchGuid: guid, Players: players, Winner: -1})
}

func TestLobbyLookupPopupAndSmurf(t *testing.T) {
	ta, me := lobbyApp(t)
	players := []game.Player{
		{Name: "Max", PrimaryID: "Epic|m|0", Team: 0},
		{Name: "Jamie", PrimaryID: me, Team: 0},
		{Name: "Rook", PrimaryID: "Epic|o1|0", Team: 1},
		{Name: "Sly", PrimaryID: "PS4|o2|0", Team: 1},
		{Name: "Bot", PrimaryID: "Unknown|0|0", Team: 1},
	}
	ta.updateState("G1", players[:2]...) // the others are still loading
	ta.updateState("G1", players...)
	ta.updateState("G1", players...)
	ta.waitFor("popup", func() bool { _, up, _ := ta.desk.lastLobby(); return up })

	p, _, _ := ta.desk.lastLobby()
	if p.Title != "Lobby · 2v2" || p.Note != "" || p.Smurf {
		t.Errorf("popup = %+v", p)
	}
	if len(p.Teams[0]) != 2 || !p.Teams[0][0].Me || p.Teams[0][0].Name != "Jamie" || len(p.Teams[1]) != 2 {
		t.Fatalf("teams = %+v, want Jamie first in blue, two in orange and no bot", p.Teams)
	}
	j := p.Teams[0][0]
	if j.Ranks[1] != (LobbyRank{Label: "C2", MMR: "1123", Tier: 17}) || j.Ranks[0].Label != "D1" || j.Ranks[2].Label != "–" || j.Wins != "1.234" {
		t.Errorf("Jamie = %+v", j)
	}
	if p.Teams[1][1].Ranks[1].Label != "GC1" || p.Teams[1][1].Wins != "" {
		t.Errorf("Sly = %+v", p.Teams[1][1])
	}
	ta.mmr.mu.Lock()
	looked := ta.mmr.looked
	ta.mmr.mu.Unlock()
	if len(looked) != 1 || len(looked[0]) != 4 {
		t.Errorf("lookups = %v, want one with the four players", looked)
	}

	ta.lobby.toggleSmurf()
	if p, _, _ := ta.desk.lastLobby(); !p.Smurf {
		t.Error("popup does not show the smurf")
	}
	ta.tracker.Handle(game.Event{Name: "MatchEnded", MatchGuid: "G1", Winner: 0})
	l, ok, _ := ta.store.Lobby("G1")
	if !ok || !l.Smurf || l.Winner != 0 || l.Size != 2 || len(l.Players) != 4 {
		t.Fatalf("stored lobby = %+v", l)
	}

	s := ta.MMRStats(0, 2)
	if s.Lobbies != 1 || s.Smurfs != 1 || s.Smurf.Wins != 1 || s.Compared != 1 || s.OppAbove != 1 || !s.MeKnown {
		t.Errorf("stats = %+v", s)
	}
	if s.OppDiff != 1225-1123 || s.MateDiff == nil || *s.MateDiff != 1050-1123 || len(s.Points) != 1 || s.Points[0].Result != 1 {
		t.Errorf("stats = %+v, opponents +102, teammate -73", s)
	}
	if s.Sizes != [3]int{0, 1, 0} || len(s.Recent) != 1 || s.Recent[0].Result != 1 {
		t.Errorf("sizes %v, recent %+v", s.Sizes, s.Recent)
	}
}

func TestLobbyLateJoinerAndToggle(t *testing.T) {
	ta, me := lobbyApp(t)
	ta.updateState("G1", game.Player{Name: "Jamie", PrimaryID: me}, game.Player{Name: "Rook", PrimaryID: "Epic|o1|0", Team: 1})
	ta.waitFor("popup", func() bool { _, up, _ := ta.desk.lastLobby(); return up })
	ta.updateState("G1", game.Player{Name: "Jamie", PrimaryID: me}, game.Player{Name: "Sly", PrimaryID: "PS4|o2|0", Team: 1})
	ta.waitFor("second lookup", func() bool { ta.mmr.mu.Lock(); defer ta.mmr.mu.Unlock(); return len(ta.mmr.looked) == 2 })
	ta.waitFor("updated popup", func() bool {
		p, _, _ := ta.desk.lastLobby()
		return len(p.Teams[1]) == 1 && p.Teams[1][0].Name == "Sly" && p.Teams[1][0].Ranks[1].Label == "GC1"
	})

	ta.lobby.toggle()
	if _, up, _ := ta.desk.lastLobby(); up {
		t.Error("toggle did not close the popup")
	}
	ta.lobby.toggle()
	if _, up, _ := ta.desk.lastLobby(); !up {
		t.Error("toggle did not open the popup")
	}
	l, _, _ := ta.store.Lobby("G1")
	if len(l.Players) != 3 || !l.Players[1].Left {
		t.Errorf("players = %+v, want Rook marked as left", l.Players)
	}
}

func TestLobbyNeedsDevMode(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.MMR = true })
	ta.mmr.signedIn = true
	ta.updateState("G1", game.Player{Name: "Rook", PrimaryID: "Epic|o1|0"})
	if _, ok, _ := ta.store.Lobby("G1"); ok {
		t.Error("lobby recorded without developer mode")
	}

	if err := ta.SetDevMode(true, "falsch"); err == nil {
		t.Error("wrong password accepted")
	}
	if _, err := ta.UpdateSettings([]byte(`{"dev_mode":true}`)); err != nil || ta.Config().DevMode {
		t.Errorf("dev_mode set through the settings: %v", err)
	}
	if err := ta.SetDevMode(true, devPassword); err != nil || !ta.Config().DevMode {
		t.Fatalf("SetDevMode() = %v", err)
	}
	if ta.settingsView().Epic == nil {
		t.Error("settings lack the Epic account in developer mode")
	}
	ta.updateState("G2", game.Player{Name: "Rook", PrimaryID: "Epic|o1|0"})
	if _, ok, _ := ta.store.Lobby("G2"); !ok {
		t.Error("lobby not recorded in developer mode")
	}
}

func TestEpicLoginKeepsToken(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.DevMode, c.MMR = true, true })
	if v := ta.epicInfo(); v.Status != "none" {
		t.Errorf("before login: %+v", v)
	}
	if _, err := ta.mmr.Login(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(ta.opts.ConfigPath)
	if err != nil || cfg.EpicToken != "refresh-abc" || cfg.EpicName != "Zweitkonto" {
		t.Errorf("saved %q %q, %v", cfg.EpicToken, cfg.EpicName, err)
	}
	if _, err := ta.UpdateSettings([]byte(`{"epic_name":"x","lobby_seconds":20}`)); err != nil || ta.Config().EpicName != "Zweitkonto" {
		t.Errorf("epic_name changed through the settings: %v", err)
	}
	if v := ta.epicInfo(); v.Status != "ok" || v.Name != "Zweitkonto" {
		t.Errorf("after login: %+v", v)
	}
}

func TestTierShort(t *testing.T) {
	for tier, want := range map[int]string{0: "Unr.", 1: "B1", 6: "S3", 7: "G1", 12: "P3", 15: "D3", 16: "C1", 19: "GC1", 21: "GC3", 22: "SSL"} {
		if got := tierShort(tier); got != want {
			t.Errorf("tierShort(%d) = %q, want %q", tier, got, want)
		}
	}
	if thousands(1234567) != "1.234.567" || thousands(999) != "999" {
		t.Error("thousands")
	}
	if ballchasingPID("XboxOne|123|0") != "xbox:123" || ballchasingPID("Steam|7656|0") != "steam:7656" {
		t.Error("ballchasingPID")
	}
}
