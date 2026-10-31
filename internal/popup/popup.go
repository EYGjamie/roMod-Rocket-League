// Package popup shows the quick-sort prompt, the lobby of the developer
// mode and notifications: on Windows as small windows that stay above the
// game without taking focus, on Linux and macOS as notifications of the
// desktop.
package popup

import (
	"fmt"
	"strings"
)

// Quick is the quick-sort prompt.
type Quick struct {
	Title   string    // replay summary
	Target  string    // folder chosen by the rule
	Options [4]string // labels of the choices 1–4, "" if unused
	Keys    [4]string // their hotkeys as shown, e.g. "Strg+Alt+1", "" if none
	Seconds int
}

// Toast is a notification with up to two actions.
type Toast struct {
	Title   string
	Body    string
	Actions []Action
	Disable func() // "don't show again", nil hides the link
}

type Action struct {
	Label string
	Run   func()
}

// Lobby is the popup with the ranks and wins of the players in the match.
type Lobby struct {
	Title   string    // e.g. "Lobby · 2v2"
	Note    string    // state of the lookup, "" when done
	Busy    bool      // the lookup runs
	Smurf   bool      // the user marked a smurf in the lobby
	Size    int       // players per team, a place for each
	Rank    int       // the playlist whose rank the popup shows, 1 to 3
	Avg     [2]string // average MMR of the teams in it, "–" if not known
	Hints   []Hint    // the hotkeys
	Seconds int       // until the popup closes again
	Teams   [2][]LobbyPlayer
}

// Hint is a hotkey as shown, e.g. Key "Strg+Alt+S" and Action "Smurf".
type Hint struct{ Key, Action string }

// LobbyPlayer is one row of the lobby popup.
type LobbyPlayer struct {
	Name  string
	Me    bool
	Ranks [3]Rank // 1s, 2s, 3s
	Wins  string  // "" if not known
}

// Rank is a rank as shown: Label "C2" and MMR "1123". An empty Label means
// the lookup has not answered yet.
type Rank struct {
	Label string
	MMR   string
	Tier  int // 0 unranked, 1 Bronze I … 22 Supersonic Legend
}

// lobbyText writes the lobby as lines, for notifications.
func lobbyText(l Lobby) string {
	var b strings.Builder
	if l.Smurf {
		b.WriteString("Smurf markiert\n")
	}
	for team, rows := range l.Teams {
		for _, p := range rows {
			b.WriteString([]string{"🔵 ", "🟠 "}[team] + p.Name)
			for i, r := range p.Ranks {
				label := r.Label
				if label == "" {
					label = "…"
				}
				fmt.Fprintf(&b, " · %ds %s", i+1, strings.TrimSpace(label+" "+r.MMR))
			}
			if p.Wins != "" {
				b.WriteString(" · " + p.Wins + " Siege")
			}
			b.WriteString("\n")
		}
	}
	if l.Note != "" {
		b.WriteString(l.Note + "\n")
	}
	return strings.TrimSpace(b.String())
}
