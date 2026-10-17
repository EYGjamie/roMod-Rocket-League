package rules

import (
	"slices"
	"testing"

	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

func TestMatch(t *testing.T) {
	rs := []store.Rule{
		{ID: 1, Enabled: true, Playlists: []string{"ranked-doubles"}, Players: []string{"steam:2", "steam:3"}, AllPlayers: true, TargetID: "trio"},
		{ID: 2, Enabled: true, Players: []string{"steam:2"}, TargetID: "mit-max"},
		{ID: 3, Enabled: false, Playlists: []string{"private"}, TargetID: "aus"},
		{ID: 4, Enabled: true, Playlists: []string{"ranked-doubles", "ranked-standard"}, TargetID: "ranked"},
		{ID: 5, Enabled: true, TargetID: "leer"},
	}
	for _, tc := range []struct {
		f    Facts
		want int64
	}{
		{Facts{"ranked-doubles", []string{"steam:2", "steam:3"}}, 1},
		{Facts{"ranked-doubles", []string{"steam:2"}}, 2},
		{Facts{"private", []string{"steam:2"}}, 2},
		{Facts{"ranked-standard", nil}, 4},
		{Facts{"private", nil}, 0},
	} {
		r, ok := Match(rs, tc.f)
		if got := r.ID; !ok && tc.want != 0 || ok && got != tc.want {
			t.Errorf("Match(%+v) = %d, %v, want %d", tc.f, got, ok, tc.want)
		}
	}
}

func TestFactsOf(t *testing.T) {
	r := store.Replay{PlaylistID: "ranked-standard", Players: []store.Player{
		{Team: "orange", PID: "steam:9"}, {Team: "blue", PID: "steam:3"}, {Team: "blue", PID: "steam:1"}, {Team: "blue", PID: "steam:2"}}}
	f := FactsOf(r, map[string]bool{"steam:1": true})
	if f.PlaylistID != "ranked-standard" || !slices.Equal(f.Mates, []string{"steam:2", "steam:3"}) {
		t.Errorf("FactsOf() = %+v", f)
	}
}

func TestDefaults(t *testing.T) {
	ds := Defaults()
	if len(ds) != 20 {
		t.Errorf("%d default rules, want 20", len(ds))
	}
	r, ok := Match(ds, Facts{PlaylistID: "ranked-solo-standard"})
	if !ok || r.TargetName != "3s Ranked" {
		t.Errorf("solo standard goes to %q", r.TargetName)
	}
	if _, ok := Match(ds, Facts{PlaylistID: "offline"}); ok {
		t.Error("offline games must go to the inbox")
	}
}

func TestSuggest(t *testing.T) {
	groups := map[string]string{"g-max": "Mit Max", "g-2s": "2s Ranked", "g-priv": "Privat & Training", "g-alt": "Alt"}
	names := map[string]string{"steam:2": "Max"}
	decisions := []store.Decision{ // newest first
		{PlaylistID: "ranked-doubles", Mates: []string{"steam:2"}, GroupID: "g-max", Manual: true},
		{PlaylistID: "ranked-doubles", Mates: []string{"steam:2"}, GroupID: "g-max", Manual: true},
		{PlaylistID: "ranked-doubles", Mates: []string{"steam:7"}, GroupID: "g-2s"},
		{PlaylistID: "private", GroupID: "g-priv", Manual: true},
		{PlaylistID: "private", GroupID: "weg"}, // deleted group
	}
	rules := []store.Rule{{Enabled: true, Playlists: []string{"ranked-doubles"}, TargetID: "g-2s"}}

	got := Suggest(Facts{"ranked-doubles", []string{"steam:2"}}, rules, decisions, groups, names, 3)
	if len(got) != 3 || got[0].GroupID != "g-2s" || got[0].Reason != "Regel" || got[1].GroupID != "g-max" {
		t.Fatalf("Suggest() = %+v", got)
	}
	if got[1].Reason != "wie 2 frühere Spiele mit Max" {
		t.Errorf("reason = %q", got[1].Reason)
	}
	got = Suggest(Facts{PlaylistID: "private"}, nil, decisions, groups, names, 2)
	if len(got) != 2 || got[0].GroupID != "g-priv" || got[0].Reason != "wie 1 frühere Spiele in Private" {
		t.Errorf("Suggest(private) = %+v", got)
	}
}

func TestSuggestRules(t *testing.T) {
	d := store.Decision{PlaylistID: "ranked-doubles", Mates: []string{"steam:2"}, GroupID: "g-max", Manual: true}
	decisions := []store.Decision{d, d, d, {PlaylistID: "private", GroupID: "g-priv", Manual: true}}
	got := SuggestRules(decisions, nil, func(string) bool { return false })
	if len(got) != 1 || got[0].Count != 3 || got[0].GroupID != "g-max" {
		t.Fatalf("SuggestRules() = %+v", got)
	}
	rule := got[0].Rule()
	if !rule.AllPlayers || !slices.Equal(rule.Players, []string{"steam:2"}) || rule.TargetID != "g-max" {
		t.Errorf("Rule() = %+v", rule)
	}
	if got := SuggestRules(decisions, []store.Rule{rule}, func(string) bool { return false }); len(got) != 0 {
		t.Errorf("suggestion despite an existing rule: %+v", got)
	}
	if got := SuggestRules(decisions, nil, func(k string) bool { return k == got[0].Key }); len(got) != 0 {
		t.Errorf("dismissed suggestion still shown: %+v", got)
	}
}

func TestPlaylistOutweighsTeammates(t *testing.T) {
	groups := map[string]string{"g-threes": "Threes", "g-priv": "Privat"}
	var decisions []store.Decision
	for range 8 {
		decisions = append(decisions, store.Decision{PlaylistID: "ranked-standard", Mates: []string{"steam:2", "steam:3"}, GroupID: "g-threes", Manual: true})
	}
	decisions = append(decisions, store.Decision{PlaylistID: "private", Mates: []string{"steam:9"}, GroupID: "g-priv", Manual: true})
	got := Suggest(Facts{"private", []string{"steam:2"}}, nil, decisions, groups, nil, 2)
	if len(got) != 2 || got[0].GroupID != "g-priv" {
		t.Errorf("Suggest() = %+v, want the private folder first", got)
	}
}
