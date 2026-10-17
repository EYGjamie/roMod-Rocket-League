package game

import (
	"io"
	"slices"
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
	err := readEvents(strings.NewReader(stream), func(e Event) { got = append(got, e) })
	if err != io.EOF {
		t.Fatalf("readEvents() error = %v, want io.EOF", err)
	}
	want := []Event{
		{Name: "RoundStarted", MatchGuid: "G1"},
		{Name: "MatchEnded", MatchGuid: "G1"},
		{Name: "MatchDestroyed"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}
