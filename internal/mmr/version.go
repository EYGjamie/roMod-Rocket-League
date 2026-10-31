package mmr

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dank/rlapi"
)

// Version is the game build PsyNet expects. It changes with every game
// update; rlapi puts the new one into psynet.go on GitHub within days, long
// before a release of this tool could include it.
type Version struct {
	Game        string `json:"game"` // e.g. 261013.63066.530866
	FeatureSet  string `json:"feature_set"`
	BuildSecret string `json:"build_secret"`
}

// VersionURL is the file of rlapi with the version of the current game.
const VersionURL = "https://raw.githubusercontent.com/dank/rlapi/master/psynet.go"

// Builtin is the version the rlapi of this build knows.
func Builtin() Version {
	p := rlapi.NewPsyNet()
	game, features := p.GetVersion()
	return Version{Game: game, FeatureSet: features, BuildSecret: p.GetBuildSecret()}
}

var (
	constPattern   = regexp.MustCompile(`(gameVersion|featureSet|buildSecret)\s*=\s*"([^"]*)"`)
	gamePattern    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	featurePattern = regexp.MustCompile(`^PrimeUpdate\w+$`)
	secretPattern  = regexp.MustCompile(`^[0-9A-Za-z]{16,128}$`)
)

// parseVersion reads the constants out of rlapi's psynet.go. They only go
// into headers, still each must look like what the game sends.
func parseVersion(src []byte) (Version, error) {
	var v Version
	for _, m := range constPattern.FindAllSubmatch(src, -1) {
		switch value := string(m[2]); string(m[1]) {
		case "gameVersion":
			v.Game = value
		case "featureSet":
			v.FeatureSet = value
		case "buildSecret":
			v.BuildSecret = value
		}
	}
	if !gamePattern.MatchString(v.Game) || !featurePattern.MatchString(v.FeatureSet) || !secretPattern.MatchString(v.BuildSecret) {
		return Version{}, fmt.Errorf("unerwartete Werte in psynet.go: %q, %q", v.Game, v.FeatureSet)
	}
	return v, nil
}

// FetchVersion reads the current version from rlapi on GitHub.
func FetchVersion(ctx context.Context) (Version, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, VersionURL, nil)
	if err != nil {
		return Version{}, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Version{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Version{}, fmt.Errorf("GitHub: HTTP %d", res.StatusCode)
	}
	src, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Version{}, err
	}
	return parseVersion(src)
}

// compareGame compares game versions like 261013.63066.530866 part by
// part; the first part is the build date.
func compareGame(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

// newest is the newer of a stored version and the built-in one, so a
// stored version from before an update of this tool does not win.
func newest(stored Version) Version {
	if b := Builtin(); stored.Game == "" || compareGame(stored.Game, b.Game) < 0 {
		return b
	}
	return stored
}

// updateVersion looks on GitHub for a newer version and reports whether it
// took one. mu is held.
func (c *Client) updateVersion(ctx context.Context) bool {
	v, err := c.opts.Fetch(ctx)
	if err != nil {
		log.Printf("MMR-Abfrage: Spielversion von rlapi nicht abrufbar: %v", err)
		return false
	}
	cur := *c.version.Load()
	if v == cur || compareGame(v.Game, cur.Game) < 0 {
		return false
	}
	log.Printf("MMR-Abfrage: neue Spielversion %s (%s) von rlapi übernommen", v.Game, v.FeatureSet)
	c.version.Store(&v)
	c.opts.SaveVersion(v)
	return true
}

// psyNet returns rlapi's client for the version in use.
func (c *Client) psyNet() *rlapi.PsyNet {
	v := c.version.Load()
	p := rlapi.NewPsyNet()
	p.SetVersion(v.Game, v.FeatureSet)
	p.SetBuildSecret(v.BuildSecret)
	return p
}

// Version returns the game version in use; it does not wait for a sign-in
// in progress.
func (c *Client) Version() Version { return *c.version.Load() }
