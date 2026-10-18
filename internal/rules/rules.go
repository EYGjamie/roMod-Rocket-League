// Package rules decides which folder (ballchasing group) a replay belongs
// in: by the user's rules, and by suggestions learned from earlier
// decisions.
package rules

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/EYGjamie/auto-replay-upload/internal/store"
)

// Playlist is a ballchasing playlist ID with its display name.
type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Playlists lists the playlists ballchasing knows, in the order the
// interface offers them.
var Playlists = []Playlist{
	{"ranked-duels", "Ranked Duels (1s)"}, {"ranked-doubles", "Ranked Doubles (2s)"},
	{"ranked-standard", "Ranked Standard (3s)"}, {"ranked-solo-standard", "Ranked Solo Standard"},
	{"unranked-duels", "Casual Duels (1s)"}, {"unranked-doubles", "Casual Doubles (2s)"},
	{"unranked-standard", "Casual Standard (3s)"}, {"unranked-chaos", "Casual Chaos (4s)"},
	{"ranked-hoops", "Ranked Hoops"}, {"hoops", "Hoops"}, {"ranked-rumble", "Ranked Rumble"}, {"rumble", "Rumble"},
	{"ranked-dropshot", "Ranked Dropshot"}, {"dropshot", "Dropshot"}, {"ranked-snowday", "Ranked Snow Day"},
	{"snowday", "Snow Day"}, {"heatseeker", "Heatseeker"}, {"dropshot-rumble", "Dropshot Rumble"},
	{"rocketlabs", "Rocket Labs"}, {"tournament", "Turniere"}, {"private", "Private"},
	{"season", "Saison (offline)"}, {"offline", "Offline"},
}

// PlaylistName returns the display name of a playlist ID.
func PlaylistName(id string) string {
	for _, p := range Playlists {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

// Defaults returns the standard rules: one folder per playlist, ranked and
// casual apart. Their groups are created on the first match.
func Defaults() []store.Rule {
	defs := []struct {
		playlists []string
		folder    string
	}{
		{[]string{"ranked-duels"}, "1s Ranked"},
		{[]string{"ranked-doubles"}, "2s Ranked"},
		{[]string{"ranked-standard", "ranked-solo-standard"}, "3s Ranked"},
		{[]string{"unranked-duels"}, "1s Casual"},
		{[]string{"unranked-doubles"}, "2s Casual"},
		{[]string{"unranked-standard"}, "3s Casual"},
		{[]string{"unranked-chaos"}, "4s Casual"},
		{[]string{"ranked-hoops"}, "Hoops Ranked"},
		{[]string{"hoops"}, "Hoops Casual"},
		{[]string{"ranked-rumble"}, "Rumble Ranked"},
		{[]string{"rumble"}, "Rumble Casual"},
		{[]string{"ranked-dropshot"}, "Dropshot Ranked"},
		{[]string{"dropshot"}, "Dropshot Casual"},
		{[]string{"ranked-snowday"}, "Snow Day Ranked"},
		{[]string{"snowday"}, "Snow Day Casual"},
		{[]string{"heatseeker"}, "Heatseeker"},
		{[]string{"dropshot-rumble"}, "Dropshot Rumble"},
		{[]string{"rocketlabs"}, "Rocket Labs"},
		{[]string{"tournament"}, "Turniere"},
		{[]string{"private"}, "Private"},
	}
	out := make([]store.Rule, len(defs))
	for i, d := range defs {
		out[i] = store.Rule{Pos: i + 1, Enabled: true, Playlists: d.playlists, TargetName: d.folder}
	}
	return out
}

// Facts are what rules look at.
type Facts struct {
	PlaylistID string
	Mates      []string // IDs of the user's teammates
}

// FactsOf extracts the facts of a replay for the user with the given IDs.
func FactsOf(r store.Replay, me map[string]bool) Facts {
	mates := r.Mates(me)
	sort.Strings(mates)
	return Facts{PlaylistID: r.PlaylistID, Mates: mates}
}

// Matches reports whether a rule applies. A rule without any condition
// matches nothing.
func Matches(r store.Rule, f Facts) bool {
	if !r.Enabled || len(r.Playlists) == 0 && len(r.Players) == 0 {
		return false
	}
	if len(r.Playlists) > 0 && !slices.Contains(r.Playlists, f.PlaylistID) {
		return false
	}
	if len(r.Players) == 0 {
		return true
	}
	hits := 0
	for _, p := range r.Players {
		if slices.Contains(f.Mates, p) {
			hits++
		}
	}
	if r.AllPlayers {
		return hits == len(r.Players)
	}
	return hits > 0
}

// Match returns the first rule that applies.
func Match(rules []store.Rule, f Facts) (store.Rule, bool) {
	for _, r := range rules {
		if Matches(r, f) {
			return r, true
		}
	}
	return store.Rule{}, false
}

// Suggestion is a folder proposed for a replay.
type Suggestion struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	score   int
}

// Suggest ranks up to n folders for a replay: the matching rule's target
// first, then folders that held similar replays, then recently used ones.
// groups maps group IDs to names (only these can be suggested), names maps
// player IDs to names for the reasons.
func Suggest(f Facts, rules []store.Rule, decisions []store.Decision, groups, names map[string]string, n int) []Suggestion {
	byGroup := map[string]*Suggestion{}
	add := func(id string, score int, reason string) {
		if _, ok := groups[id]; !ok || id == "" {
			return
		}
		s := byGroup[id]
		if s == nil {
			s = &Suggestion{GroupID: id, Name: groups[id]}
			byGroup[id] = s
		}
		s.score += score
		if s.Reason == "" {
			s.Reason = reason
		}
	}
	if r, ok := Match(rules, f); ok && r.TargetID != "" {
		add(r.TargetID, 1000, "Regel")
	}

	// The playlist matters most; teammates refine it. The same teammates in
	// another playlist count only a little.
	type tally struct{ playlistMates, playlistSomeMates, playlist, mates, recent int }
	counts := map[string]*tally{}
	for i, d := range decisions {
		if d.GroupID == "" {
			continue
		}
		t := counts[d.GroupID]
		if t == nil {
			t = &tally{}
			counts[d.GroupID] = t
		}
		samePlaylist := d.PlaylistID == f.PlaylistID
		overlap := len(f.Mates) > 0 && overlaps(d.Mates, f.Mates)
		switch {
		case samePlaylist && len(f.Mates) > 0 && slices.Equal(d.Mates, f.Mates):
			t.playlistMates++
		case samePlaylist && overlap:
			t.playlistSomeMates++
		case samePlaylist:
			t.playlist++
		case overlap:
			t.mates++
		}
		if i < 20 {
			t.recent++
		}
	}
	for id, t := range counts {
		// weak signals saturate: a folder used a lot recently must not beat one
		// that fits the playlist
		score := 10*t.playlistMates + 6*t.playlistSomeMates + 3*t.playlist + min(t.mates, 2) + min(t.recent, 1)
		if score == 0 {
			continue
		}
		var reason string
		switch {
		case t.playlistMates+t.playlistSomeMates > 0:
			reason = fmt.Sprintf("wie %d frühere Spiele mit %s", t.playlistMates+t.playlistSomeMates, mateNames(f.Mates, names))
		case t.playlist > 0:
			reason = fmt.Sprintf("wie %d frühere Spiele in %s", t.playlist, PlaylistName(f.PlaylistID))
		default:
			reason = "zuletzt genutzt"
		}
		add(id, score, reason)
	}

	out := make([]Suggestion, 0, len(byGroup))
	for _, s := range byGroup {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].Name < out[j].Name
	})
	return out[:min(n, len(out))]
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

func mateNames(ids []string, names map[string]string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = names[id]
		if out[i] == "" {
			out[i] = id
		}
	}
	return strings.Join(out, " und ")
}

// RuleSuggestion proposes a rule for something the user keeps doing by
// hand.
type RuleSuggestion struct {
	Key        string   `json:"key"` // for dismissing it
	PlaylistID string   `json:"playlist_id"`
	Mates      []string `json:"mates"`
	GroupID    string   `json:"group_id"`
	Count      int      `json:"count"`
}

// SuggestRules looks for at least three manual decisions with the same
// playlist and teammates into the same group that no rule covers yet.
func SuggestRules(decisions []store.Decision, rules []store.Rule, ignored func(key string) bool) []RuleSuggestion {
	byKey := map[string]*RuleSuggestion{}
	for _, d := range decisions {
		if !d.Manual || d.GroupID == "" {
			continue
		}
		key := "regel:" + d.PlaylistID + "|" + strings.Join(d.Mates, ",") + "|" + d.GroupID
		s := byKey[key]
		if s == nil {
			s = &RuleSuggestion{Key: key, PlaylistID: d.PlaylistID, Mates: d.Mates, GroupID: d.GroupID}
			byKey[key] = s
		}
		s.Count++
	}
	var out []RuleSuggestion
	for key, s := range byKey {
		if s.Count < 3 || ignored(key) {
			continue
		}
		if r, ok := Match(rules, Facts{PlaylistID: s.PlaylistID, Mates: s.Mates}); ok && r.TargetID == s.GroupID {
			continue
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Rule turns a suggestion into a rule. With teammates it gets a condition
// on them as well, so it sits before the plain playlist rules.
func (s RuleSuggestion) Rule() store.Rule {
	r := store.Rule{Enabled: true, Playlists: []string{s.PlaylistID}, TargetID: s.GroupID}
	if len(s.Mates) > 0 {
		r.Players, r.AllPlayers = s.Mates, true
	}
	return r
}
