package mmr

import (
	"context"
	"strings"
	"testing"
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

	if !c.updateVersion(context.Background(), versionEvery) || c.Version() != newer || len(saved) != 1 {
		t.Errorf("newer version not taken: %+v, saved %v", c.Version(), saved)
	}
	if c.updateVersion(context.Background(), versionEvery) || calls != 1 {
		t.Errorf("looked again within the interval (%d calls)", calls)
	}
	fetched = older
	if c.updateVersion(context.Background(), 0) || c.Version() != newer {
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
