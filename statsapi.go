package main

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
		MatchGuid string `json:"MatchGuid"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			return Event{}, err
		}
	}
	return Event{Name: env.Event, MatchGuid: d.MatchGuid}, nil
}

// readEvents decodes the concatenated JSON stream from r and calls handle for
// every event except the periodic UpdateState.
func readEvents(r io.Reader, handle func(Event)) error {
	dec := json.NewDecoder(r)
	for {
		var env envelope
		if err := dec.Decode(&env); err != nil {
			return err
		}
		if env.Event == "UpdateState" {
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

// listenStatsAPI connects to the game's TCP Stats API and reconnects until
// ctx is done. handle is only ever called from this goroutine.
func listenStatsAPI(ctx context.Context, addr string, handle func(Event)) {
	var d net.Dialer
	waiting := false
	for {
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			log.Printf("Mit der Stats API verbunden (%s)", addr)
			waiting = false
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			err = readEvents(conn, handle)
			stop()
			conn.Close()
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
