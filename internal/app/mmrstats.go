package app

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/store"
)

// The analysis of the stored lobbies: how often a smurf was in the lobby,
// how the user's MMR moved and where the opponents stood against it.

type winLoss struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
}

func (w *winLoss) add(result int) {
	switch result {
	case 1:
		w.Wins++
	case 0:
		w.Losses++
	}
}

// mmrPoint is one lobby in the chart.
type mmrPoint struct {
	GUID   string    `json:"guid"`
	At     time.Time `json:"at"`
	Me     float64   `json:"me"`
	Opp    *float64  `json:"opp"`   // average of the opponents, nil if none known
	Mates  *float64  `json:"mates"` // average of the teammates
	Result int       `json:"result"`
	Smurf  bool      `json:"smurf"`
}

// lobbyRow is a lobby in the list, with what the replays add.
type lobbyRow struct {
	store.Lobby
	Playlist string `json:"playlist"` // from the replay with the same match, "" if none
	Result   int    `json:"result"`   // 1 win, 0 loss, -1 not known
}

type mmrStatsView struct {
	Size     int        `json:"size"`  // players per team the numbers are for
	Sizes    [3]int     `json:"sizes"` // lobbies per team size in the period
	Lobbies  int        `json:"lobbies"`
	Smurfs   int        `json:"smurfs"`
	Smurf    winLoss    `json:"smurf_record"` // with a smurf in the lobby
	Clean    winLoss    `json:"clean_record"` // without
	Compared int        `json:"compared"`     // lobbies with the own and the opponents' MMR
	OppAbove int        `json:"opp_above"`    // of those, opponents on average above the user
	OppDiff  float64    `json:"opp_diff"`     // opponents minus own, averaged
	MateDiff *float64   `json:"mate_diff"`    // teammates minus own, nil without teammates
	MeKnown  bool       `json:"me_known"`     // the user was found in a lobby
	Points   []mmrPoint `json:"points"`
	Recent   []lobbyRow `json:"recent"` // newest first
}

// maxRecent limits the list of lobbies.
const maxRecent = 50

// standardPlaylists are the playlists the analysis is about. A lobby whose
// replay shows another one (Hoops, Rumble, a private match) is left out;
// without a replay it counts.
var standardPlaylists = map[string]bool{"ranked-duels": true, "ranked-doubles": true, "ranked-standard": true,
	"unranked-duels": true, "unranked-doubles": true, "unranked-standard": true}

// MMRStats evaluates the lobbies of the last days (0: all) with size
// players per team.
func (a *App) MMRStats(days, size int) mmrStatsView {
	var since time.Time
	if days > 0 {
		since = a.now().AddDate(0, 0, -days)
	}
	lobbies, _ := a.store.Lobbies(since)
	me := a.myIDs()
	playlists := map[string]string{}
	for _, r := range a.visible() {
		if r.MatchGUID != "" {
			playlists[strings.ToLower(r.MatchGUID)] = r.PlaylistID
		}
	}

	v := mmrStatsView{Size: size, Points: []mmrPoint{}, Recent: []lobbyRow{}}
	var oppSum, mateSum float64
	var mates int
	for _, l := range lobbies {
		if pl := playlists[strings.ToLower(l.GUID)]; pl != "" && !standardPlaylists[pl] {
			continue
		}
		if l.Size >= 1 && l.Size <= 3 {
			v.Sizes[l.Size-1]++
		}
		if l.Size != size {
			continue
		}
		self := slices.IndexFunc(l.Players, func(p store.LobbyPlayer) bool { return p.Me || me[ballchasingPID(p.ID)] })
		result := -1
		if self >= 0 && l.Winner >= 0 {
			result = 0
			if l.Players[self].Team == l.Winner {
				result = 1
			}
		}
		v.Lobbies++
		v.MeKnown = v.MeKnown || self >= 0
		if l.Smurf {
			v.Smurfs++
			v.Smurf.add(result)
		} else {
			v.Clean.add(result)
		}
		v.Recent = append(v.Recent, lobbyRow{Lobby: l, Playlist: playlists[strings.ToLower(l.GUID)], Result: result})
		if self < 0 || l.Players[self].Ranks[size-1] == nil {
			continue
		}
		own := l.Players[self].Ranks[size-1].MMR
		pt := mmrPoint{GUID: l.GUID, At: l.Started, Me: own, Result: result, Smurf: l.Smurf}
		pt.Opp = average(l.Players, size, func(p store.LobbyPlayer) bool { return p.Team != l.Players[self].Team })
		pt.Mates = average(l.Players, size, func(p store.LobbyPlayer) bool { return p.Team == l.Players[self].Team && p.ID != l.Players[self].ID })
		if pt.Opp != nil {
			v.Compared++
			oppSum += *pt.Opp - own
			if *pt.Opp > own {
				v.OppAbove++
			}
		}
		if pt.Mates != nil {
			mates++
			mateSum += *pt.Mates - own
		}
		v.Points = append(v.Points, pt)
	}
	if v.Compared > 0 {
		v.OppDiff = oppSum / float64(v.Compared)
	}
	if mates > 0 {
		d := mateSum / float64(mates)
		v.MateDiff = &d
	}
	slices.Reverse(v.Recent)
	if len(v.Recent) > maxRecent {
		v.Recent = v.Recent[:maxRecent]
	}
	return v
}

// average is the mean MMR in the playlist for size of the players that
// match, nil if none of them has one.
func average(players []store.LobbyPlayer, size int, match func(store.LobbyPlayer) bool) *float64 {
	var sum float64
	n := 0
	for _, p := range players {
		if match(p) && p.Ranks[size-1] != nil && p.Ranks[size-1].Tier > 0 {
			sum += p.Ranks[size-1].MMR
			n++
		}
	}
	if n == 0 {
		return nil
	}
	avg := sum / float64(n)
	return &avg
}

func (a *App) apiMMR(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 || size > 3 {
		size = 2
	}
	writeJSON(w, http.StatusOK, a.MMRStats(days, size))
}
