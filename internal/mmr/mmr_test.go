package mmr

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dank/rlapi"
)

func TestParseCode(t *testing.T) {
	for in, want := range map[string]string{
		"0123abcd":        "0123abcd",
		" \"0123abcd\"\n": "0123abcd",
		`{"warning":"Do not share this code","redirectUrl":"https://localhost/launcher/authorized","authorizationCode":"9f8e7d6c","exchangeCode":null,"sid":null}`: "9f8e7d6c",
		"": "",
	} {
		if got := parseCode(in); got != want {
			t.Errorf("parseCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyIgnoresCaseAndSuffix(t *testing.T) {
	if key("Steam|7656|0") != key("steam|7656") || key("Epic|a|0") == key("Epic|b|0") {
		t.Error("key")
	}
}

func TestShownMMR(t *testing.T) {
	// 25 is a new player, 81.8645 the best 1s player in rlapi's REQUESTS.md
	for mu, want := range map[float64]float64{25: 600, 81.8645: 1737.29} {
		if got := shownMMR(mu); got < want-0.01 || got > want+0.01 {
			t.Errorf("shownMMR(%v) = %v, want %v", mu, got, want)
		}
	}
}

// psynetSource is the start of rlapi's psynet.go as on GitHub.
const psynetSource = `package rlapi

const (
	baseURL      = "https://api.rlpp.psynet.gg/rpc"
	gameVersion  = "261013.63066.530866"
	featureSet   = "PrimeUpdate60_FFE7A62B41040BB67F8829FD67F2EE8A"
	buildSecret  = "76656a5b51ec57a13549eef732c8a8367638aa3a7989bade1f7e4e5f93d57625"
	psySigKey    = "c338bd36fb8c42b1a431d30add939fc7"
)`

func TestParseVersion(t *testing.T) {
	v, err := parseVersion([]byte(psynetSource))
	want := Version{Game: "261013.63066.530866", FeatureSet: "PrimeUpdate60_FFE7A62B41040BB67F8829FD67F2EE8A",
		BuildSecret: "76656a5b51ec57a13549eef732c8a8367638aa3a7989bade1f7e4e5f93d57625"}
	if err != nil || v != want {
		t.Errorf("parseVersion() = %+v, %v", v, err)
	}
	for _, bad := range []string{
		strings.Replace(psynetSource, "261013.63066.530866", "26.1 beta", 1),
		strings.Replace(psynetSource, "PrimeUpdate60_", "Prime\nUpdate", 1),
		strings.Replace(psynetSource, `buildSecret  = "76656a`, `buildSecret  = "x\" -H 'a: b`, 1),
		"<html>Not Found</html>",
	} {
		if v, err := parseVersion([]byte(bad)); err == nil {
			t.Errorf("parseVersion accepted %+v", v)
		}
	}
}

func TestCompareGame(t *testing.T) {
	if compareGame("261013.63066.530866", "261001.2867.529371") <= 0 || compareGame("261001.2867.529371", "261013.1.1") >= 0 ||
		compareGame("261013.63066.530866", "261013.63066.530866") != 0 || compareGame("261013.9.1", "261013.10.1") >= 0 {
		t.Error("compareGame")
	}
}

func TestUpdateVersion(t *testing.T) {
	builtin := Builtin()
	older := Version{Game: "200101.1.1", FeatureSet: "PrimeUpdate1", BuildSecret: "0123456789abcdef"}
	newer := Version{Game: "991231.1.1", FeatureSet: "PrimeUpdate99", BuildSecret: "fedcba9876543210"}
	var saved []Version
	fetched := newer
	calls := 0
	c := New(Options{Save: func(string, string) {}, Version: older, SaveVersion: func(v Version) { saved = append(saved, v) },
		Fetch: func(context.Context) (Version, error) { calls++; return fetched, nil }})
	if c.Version() != builtin {
		t.Errorf("stored older version %+v won over the built-in one", c.Version())
	}

	if !c.updateVersion(context.Background()) || c.Version() != newer || len(saved) != 1 || calls != 1 {
		t.Errorf("newer version not taken: %+v, saved %v", c.Version(), saved)
	}
	fetched = older
	if c.updateVersion(context.Background()) || c.Version() != newer {
		t.Errorf("went back to an older version: %+v", c.Version())
	}
	if p := c.psyNet(); p == nil {
		t.Fatal("psyNet() = nil")
	} else if game, features := p.GetVersion(); game != newer.Game || features != newer.FeatureSet || p.GetBuildSecret() != newer.BuildSecret {
		t.Errorf("PsyNet uses %s %s", game, features)
	}
	if c := New(Options{Save: func(string, string) {}, Version: newer}); c.Version() != newer {
		t.Errorf("stored newer version not used: %+v", c.Version())
	}
}

func TestVersionLookOncePerGameStart(t *testing.T) {
	calls := 0
	c := New(Options{Save: func(string, string) {}, Token: "refresh",
		Fetch: func(context.Context) (Version, error) { calls++; return Version{}, errors.New("offline") }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // stops each connection right after the look, before Epic
	connect := func(ahead bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.connectLocked(ctx, ahead)
	}

	connect(true)
	if calls != 0 {
		t.Fatal("looked on GitHub without a game start")
	}
	c.GameStarted()
	connect(false)
	if calls != 0 {
		t.Fatal("looked on GitHub while a lookup waits")
	}
	connect(true)
	connect(true)
	if calls != 1 {
		t.Errorf("%d looks after one game start, want 1", calls)
	}
}

// fakePsyNet answers like PsyNet: wins for the players in batch, and one
// by one the values in single, an error for the others.
type fakePsyNet struct {
	batch    map[rlapi.PlayerID]float64
	batchErr error
	single   map[rlapi.PlayerID]string

	mu    sync.Mutex
	asked []rlapi.PlayerID // one by one
}

func (f *fakePsyNet) GetPlayersSkills(ctx context.Context, ids []rlapi.PlayerID) ([]rlapi.PlayerWithSkills, error) {
	var out []rlapi.PlayerWithSkills
	for _, id := range ids {
		out = append(out, rlapi.PlayerWithSkills{PlayerID: id, Skills: []rlapi.Skill{{Playlist: 11, Tier: 17, MMR: 50}}})
	}
	return out, nil
}

func (f *fakePsyNet) GetStatLeaderboardRankForUsers(ctx context.Context, stat string, ids []rlapi.PlayerID) (*rlapi.GetStatLeaderboardRankForUsersResponse, error) {
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	res := &rlapi.GetStatLeaderboardRankForUsersResponse{LeaderboardID: stat}
	for id, n := range f.batch {
		res.Players = append(res.Players, rlapi.StatLeaderboardRankPlayer{PlayerID: id, Value: n})
	}
	return res, nil
}

func (f *fakePsyNet) GetStatLeaderboardValueForUser(ctx context.Context, stat string, id rlapi.PlayerID) (*rlapi.GetStatLeaderboardValueForUserResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, id)
	v, ok := f.single[id]
	if !ok {
		return nil, errors.New("ServiceUnavailable")
	}
	return &rlapi.GetStatLeaderboardValueForUserResponse{LeaderboardID: stat, Value: v}, nil
}

func TestLookupAsksForMissingWinsOneByOne(t *testing.T) {
	ids := []string{"Steam|7656|0", "Epic|b|0", "PS4|c|0", "Switch|d|0"}
	rpc := &fakePsyNet{
		batch:  map[rlapi.PlayerID]float64{"steam|7656": 1234}, // spelt otherwise, still the player asked
		single: map[rlapi.PlayerID]string{"Epic|b|0": "42", "Switch|d|0": ""},
	}
	got, err := lookup(context.Background(), rpc, ids)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Player{
		"Steam|7656|0": {Wins: 1234},
		"Epic|b|0":     {Wins: 42},
		"PS4|c|0":      {Wins: -1, WinsFailed: true},
		"Switch|d|0":   {Wins: -1},
	}
	for id, w := range want {
		p := got[id]
		if p.Wins != w.Wins || p.WinsFailed != w.WinsFailed || p.Ranks[1] == nil || p.Ranks[1].MMR != 1100 {
			t.Errorf("%s = %+v, want wins %d, failed %v and the 2s rank", id, p, w.Wins, w.WinsFailed)
		}
	}
	slices.Sort(rpc.asked)
	if !slices.Equal(rpc.asked, []rlapi.PlayerID{"Epic|b|0", "PS4|c|0", "Switch|d|0"}) {
		t.Errorf("asked one by one: %v, want those the answer lacks", rpc.asked)
	}

	rpc = &fakePsyNet{batchErr: errors.New("RateLimited"), single: map[rlapi.PlayerID]string{"Steam|7656|0": "7", "Epic|b|0": "8", "PS4|c|0": "9", "Switch|d|0": "10"}}
	got, err = lookup(context.Background(), rpc, ids)
	if err != nil || len(rpc.asked) != 4 || got["Switch|d|0"].Wins != 10 || got["Steam|7656|0"].Wins != 7 {
		t.Errorf("after a failed question: %+v, %v, asked %v", got, err, rpc.asked)
	}
}
