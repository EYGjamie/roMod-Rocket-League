package game

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestReadEvents(t *testing.T) {
	// Concatenated without separators, Data both as string (what the game
	// sends) and as object (what the docs show).
	stream := `{"Event":"UpdateState","Data":"{\"MatchGuid\":\"G1\",\"Players\":[]}"}` +
		`{"Event":"RoundStarted","Data":"{\"MatchGuid\":\"G1\"}"}` + "\n" +
		`{"Event":"GoalScored","Data":"kaputt"}` +
		`{"Event":"MatchEnded","Data":{"MatchGuid":"G1","WinnerTeamNum":0}}` +
		`{"Event":"MatchDestroyed","Data":"{}"}`

	var got []Event
	err := readEvents(strings.NewReader(stream), func(e Event) { got = append(got, e) }, nil)
	if err != io.EOF {
		t.Fatalf("readEvents() error = %v, want io.EOF", err)
	}
	want := []Event{
		{Name: "RoundStarted", MatchGuid: "G1", Winner: -1},
		{Name: "MatchEnded", MatchGuid: "G1", Winner: 0},
		{Name: "MatchDestroyed", Winner: -1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestReadEventsPlayers(t *testing.T) {
	stream := `{"Event":"UpdateState","Data":"{\"MatchGuid\":\"G1\",\"Players\":[` +
		`{\"Name\":\"Jamie\",\"PrimaryId\":\"Steam|7656|0\",\"Shortcut\":1,\"TeamNum\":0,\"Boost\":33},` +
		`{\"Name\":\"Rook\",\"PrimaryId\":\"Epic|abc|0\",\"TeamNum\":1}],\"Game\":{\"Arena\":\"Stadium_P\"}}"}`

	var got []Event
	readEvents(strings.NewReader(stream+stream), func(e Event) { got = append(got, e) }, func() bool { return len(got) == 0 })
	want := []Event{{Name: "UpdateState", MatchGuid: "G1", Winner: -1, Players: []Player{
		{Name: "Jamie", PrimaryID: "Steam|7656|0", Team: 0},
		{Name: "Rook", PrimaryID: "Epic|abc|0", Team: 1},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v (the second UpdateState is skipped)", got, want)
	}
}
