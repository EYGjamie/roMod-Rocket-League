package store

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "Ordner mit Leerzeichen", "romod.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x y", "a.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.SetMeta("cursor", "2026-10-01")
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.Meta("cursor"); got != "2026-10-01" {
		t.Errorf("Meta after reopen = %q", got)
	}
}

func TestFiles(t *testing.T) {
	s := open(t)
	if err := s.ImportLegacy(map[string]string{"alt.replay": "", "hoch.replay": "abc"}); err != nil {
		t.Fatal(err)
	}
	f, ok, err := s.File("hoch.replay")
	if err != nil || !ok || f.State != Done || f.ReplayID != "abc" {
		t.Fatalf("File(hoch) = %+v, %v, %v", f, ok, err)
	}
	next := time.Now().Add(time.Minute).Truncate(time.Second)
	if err := s.PutFile(File{Name: "neu.replay", Dir: `C:\Demos`, State: Retry, Attempts: 1, Error: "kaputt", NextTry: next}); err != nil {
		t.Fatal(err)
	}
	f, _, _ = s.File("neu.replay")
	if f.Attempts != 1 || !f.NextTry.Equal(next) || f.Added.IsZero() || f.Dir != `C:\Demos` {
		t.Errorf("File(neu) = %+v", f)
	}
	added := f.Added
	f.State = Done
	s.PutFile(f)
	if f, _, _ := s.File("neu.replay"); f.State != Done || !f.Added.Equal(added) {
		t.Errorf("update changed Added or lost state: %+v", f)
	}

	retry, _ := s.Files(Retry, Seeded)
	if len(retry) != 1 || retry[0].Name != "alt.replay" {
		t.Errorf("Files(retry, seeded) = %+v", retry)
	}
	counts, _ := s.CountFiles()
	if counts[Done] != 2 || counts[Seeded] != 1 {
		t.Errorf("CountFiles() = %v", counts)
	}
	names, _ := s.FileNames()
	if len(names) != 3 || !names["alt.replay"] {
		t.Errorf("FileNames() = %v", names)
	}
}

func TestReplays(t *testing.T) {
	s := open(t)
	me := map[string]bool{"steam:1": true}
	r := Replay{ID: "r1", Own: true, Title: "Eins", Date: time.Unix(1_700_000_000, 0), PlaylistID: "ranked-doubles",
		BlueGoals: 3, OrangeGoals: 1, File: "eins.replay",
		Players: []Player{{Team: "blue", Name: "Jamie", PID: "steam:1", Score: 400}, {Team: "blue", Name: "Max", PID: "steam:2"}, {Team: "orange", Name: "Gegner", PID: "epic:x"}}}
	if err := s.PutReplay(r); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := s.Replay("r1")
	if !ok || got.Title != "Eins" || !got.Date.Equal(r.Date) || len(got.Players) != 3 || got.HasDetail {
		t.Fatalf("Replay(r1) = %+v", got)
	}
	if got.Result(me) != "win" || got.MyTeam(me) != "blue" || !slices.Equal(got.Mates(me), []string{"steam:2"}) {
		t.Errorf("result %q, team %q, mates %v", got.Result(me), got.MyTeam(me), got.Mates(me))
	}
	if got.Result(map[string]bool{"epic:x": true}) != "loss" || got.Result(map[string]bool{"steam:9": true}) != "" {
		t.Error("Result for the other team or a stranger is wrong")
	}

	detailed := r
	detailed.File = ""
	detailed.MatchGUID = "G1"
	detailed.Players = []Player{{Team: "blue", Name: "Jamie", PID: "steam:1", Core: true, Goals: 2, Shots: 5}}
	if err := s.PutDetail(detailed, []byte(`{"id":"r1"}`)); err != nil {
		t.Fatal(err)
	}
	// a later list sync carries no statistics and must not erase them
	s.PutReplay(Replay{ID: "r1", Own: true, Title: "Eins (neu)", GroupID: "g1", GroupName: "Liga",
		Players: []Player{{Team: "blue", Name: "Jamie", PID: "steam:1", Score: 400}}})
	got, _, _ = s.Replay("r1")
	if got.Title != "Eins (neu)" || got.File != "eins.replay" || got.MatchGUID != "G1" || !got.HasDetail || !got.Players[0].Core || got.GroupName != "Liga" {
		t.Errorf("after list sync: %+v", got)
	}
	if raw, _ := s.Detail("r1"); string(raw) != `{"id":"r1"}` {
		t.Errorf("Detail(r1) = %s", raw)
	}

	s.PutReplay(Replay{ID: "r2", Date: time.Unix(1_700_100_000, 0)})
	if ids, _ := s.WithoutDetail(10); !slices.Equal(ids, []string{"r2"}) {
		t.Errorf("WithoutDetail() = %v", ids)
	}
	s.SetDupOf("r2", "r1")
	if ids, _ := s.WithoutDetail(10); len(ids) != 0 {
		t.Errorf("hidden duplicate still waits for statistics: %v", ids)
	}
	s.SetReplayGroup("r2", "g2", "Cup")
	s.SetVisibility("r2", "public")
	all, _ := s.Replays()
	if len(all) != 2 || all[0].ID != "r2" || all[0].GroupName != "Cup" || all[0].Visibility != "public" || all[0].DupOf != "r1" {
		t.Errorf("Replays() = %+v", all)
	}
	s.DeleteReplay("r2")
	if _, ok, _ := s.Replay("r2"); ok {
		t.Error("r2 still there")
	}
}

func TestGroupsRulesDecisions(t *testing.T) {
	s := open(t)
	s.ReplaceGroups([]Group{{ID: "g1", Name: "liga"}, {ID: "g2", Name: "Cup", Parent: "g1", Direct: 2}})
	s.PutGroup(Group{ID: "g3", Name: "Ältere"})
	groups, _ := s.Groups()
	if len(groups) != 3 || groups[0].Name != "Cup" || groups[0].Parent != "g1" || groups[0].Direct != 2 {
		t.Errorf("Groups() = %+v", groups)
	}

	a := Rule{Enabled: true, Playlists: []string{"ranked-doubles"}, TargetName: "2s Ranked"}
	b := Rule{Enabled: true, Players: []string{"steam:2"}, TargetID: "g2"}
	s.PutRule(&a)
	s.PutRule(&b)
	if a.ID == 0 || b.Pos <= a.Pos {
		t.Fatalf("PutRule ids/pos: %+v %+v", a, b)
	}
	s.OrderRules([]int64{b.ID, a.ID})
	a.TargetID = "g9"
	rules, _ := s.Rules()
	if rules[0].ID != b.ID || rules[1].ID != a.ID {
		t.Errorf("order after OrderRules = %+v", rules)
	}
	rules[1].TargetID = "g9"
	s.PutRule(&rules[1])
	s.DeleteRule(b.ID)
	rules, _ = s.Rules()
	if len(rules) != 1 || rules[0].TargetID != "g9" || !slices.Equal(rules[0].Playlists, []string{"ranked-doubles"}) {
		t.Errorf("Rules() = %+v", rules)
	}

	s.PutDecision(Decision{ReplayID: "r1", PlaylistID: "private", Mates: []string{"steam:3", "steam:2"}, GroupID: "g1", Manual: true})
	s.PutDecision(Decision{ReplayID: "r1", PlaylistID: "private", GroupID: "g2"})
	ds, _ := s.Decisions()
	if len(ds) != 1 || ds[0].GroupID != "g2" || ds[0].Manual {
		t.Errorf("Decisions() = %+v", ds)
	}

	s.SetIdentities([]Identity{{PID: "steam:1", Name: "Jamie", Confirmed: true}})
	if ids, _ := s.Identities(); len(ids) != 1 || !ids[0].Confirmed {
		t.Errorf("Identities() = %+v", ids)
	}
	s.PutBorrow(Borrow{Name: "bc-r1.replay", ReplayID: "r1"})
	s.PutBorrow(Borrow{Name: "bc-r1.replay", ReplayID: "r1", Keep: true})
	if bs, _ := s.Borrows(); len(bs) != 1 || !bs[0].Keep {
		t.Errorf("Borrows() = %+v", bs)
	}
	s.DeleteBorrow("bc-r1.replay")
	if bs, _ := s.Borrows(); len(bs) != 0 {
		t.Errorf("Borrows() after delete = %+v", bs)
	}
	s.Ignore("vorschlag-1")
	if !s.Ignored("vorschlag-1") || s.Ignored("vorschlag-2") {
		t.Error("Ignored() wrong")
	}
}

func TestLobbies(t *testing.T) {
	s := open(t)
	day := time.Date(2026, 10, 20, 20, 0, 0, 0, time.Local)
	l := Lobby{GUID: "G1", Started: day, Size: 2, Winner: -1, Players: []LobbyPlayer{
		{ID: "Steam|1|0", Name: "Jamie", Me: true, Wins: 1200, Ranks: [3]*Rank{nil, {Tier: 16, Div: 2, MMR: 1110}, nil}},
		{ID: "Epic|x|0", Name: "Rook", Team: 1, Wins: -1},
	}}
	if err := s.PutLobby(l); err != nil {
		t.Fatal(err)
	}
	l.Winner = 1
	s.PutLobby(l)
	s.PutLobby(Lobby{GUID: "G0", Started: day.Add(-time.Hour), Size: 1})
	if err := s.SetSmurf("G1", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSmurf("nope", true); err == nil {
		t.Error("SetSmurf of a missing lobby succeeded")
	}
	got, ok, err := s.Lobby("G1")
	if err != nil || !ok || !got.Smurf || got.Winner != 1 || len(got.Players) != 2 || got.Players[0].Ranks[1].MMR != 1110 || got.Players[1].Ranks[1] != nil {
		t.Errorf("Lobby(G1) = %+v, %v, %v", got, ok, err)
	}
	list, _ := s.Lobbies(day.Add(-2 * time.Hour))
	if len(list) != 2 || list[0].GUID != "G0" {
		t.Errorf("Lobbies() = %+v, want G0, G1", list)
	}
	if list, _ := s.Lobbies(day.Add(-time.Minute)); len(list) != 1 {
		t.Errorf("Lobbies(since) = %d lobbies, want 1", len(list))
	}
}
