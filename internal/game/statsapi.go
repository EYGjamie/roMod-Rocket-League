// Package game talks to Rocket League: it reads the Stats API, follows
// matches and holds the save-replay key once per match.
package game

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"time"
)

// Event is a Stats API event reduced to the fields this tool needs.
type Event struct {
	Name      string
	MatchGuid string
	Players   []Player // UpdateState only
	Winner    int      // MatchEnded: WinnerTeamNum, -1 if missing
}

// Player is a player of the match as UpdateState lists them.
type Player struct {
	Name      string `json:"Name"`
	PrimaryID string `json:"PrimaryId"` // e.g. Steam|76561197960287930|0
	Team      int    `json:"TeamNum"`   // 0 blue, 1 orange
}

type envelope struct {
	Event string          `json:"Event"`
	Data  json.RawMessage `json:"Data"`
}

// decodeEvent extracts the MatchGuid. The docs show Data as an object, but
// the game sends it as a JSON-encoded string, so both are accepted.
func decodeEvent(env envelope) (Event, error) {
	data := env.Data
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return Event{}, err
		}
		data = []byte(s)
	}
	var d struct {
		MatchGuid string   `json:"MatchGuid"`
		Players   []Player `json:"Players"`
		Winner    *int     `json:"WinnerTeamNum"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			return Event{}, err
		}
	}
	ev := Event{Name: env.Event, MatchGuid: d.MatchGuid, Players: d.Players, Winner: -1}
	if d.Winner != nil {
		ev.Winner = *d.Winner
	}
	return ev, nil
}

// readEvents decodes the concatenated JSON stream from r and calls handle for
// every event. The periodic UpdateState only goes on while lobby says so, as
// only the lobby lookup needs its player list.
func readEvents(r io.Reader, handle func(Event), lobby func() bool) error {
	dec := json.NewDecoder(r)
	for {
		var env envelope
		if err := dec.Decode(&env); err != nil {
			return err
		}
		if env.Event == "UpdateState" && (lobby == nil || !lobby()) {
			continue
		}
		ev, err := decodeEvent(env)
		if err != nil {
			log.Printf("Stats API: %s nicht lesbar: %v", env.Event, err)
			continue
		}
		handle(ev)
	}
}

// ListenStatsAPI connects to the game's TCP Stats API and reconnects until
// ctx is done. addr is asked before every attempt, so a changed setting
// applies on the next reconnect. handle is only ever called from this
// goroutine; connected, if not nil, reports connects and disconnects.
// UpdateState only reaches handle while lobby, if not nil, returns true.
func ListenStatsAPI(ctx context.Context, addrOf func() string, handle func(Event), connected func(bool), lobby func() bool) {
	var d net.Dialer
	waiting := false
	for {
		addr := addrOf()
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			log.Printf("Mit der Stats API verbunden (%s)", addr)
			waiting = false
			if connected != nil {
				connected(true)
			}
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			err = readEvents(conn, handle, lobby)
			stop()
			conn.Close()
			if connected != nil {
				connected(false)
			}
			if ctx.Err() == nil {
				log.Printf("Verbindung zur Stats API getrennt: %v", err)
			}
		} else if !waiting && ctx.Err() == nil {
			log.Printf("Warte auf Rocket League (Stats API %s nicht erreichbar) …", addr)
			waiting = true
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}
