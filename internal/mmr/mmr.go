// Package mmr looks up the ranks and all-time wins of Rocket League players
// on Psyonix's servers. It signs in with an Epic account, meant to be a
// second one, through the reverse-engineered API of github.com/dank/rlapi.
package mmr

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dank/rlapi"
)

// Playlists are the ranked playlists in the order of Player.Ranks: 1s, 2s
// and 3s.
var Playlists = [3]rlapi.PlaylistID{10, 11, 13}

// ErrSignedOut means there is no Epic account or Epic no longer accepts its
// sign-in.
var ErrSignedOut = errors.New("nicht bei Epic angemeldet")

// Skill is a player's rank in one playlist.
type Skill struct {
	Tier   int // 0 unranked, 1 Bronze I … 22 Supersonic Legend
	Div    int // 0–3
	MMR    float64
	Played int // matches this season
}

// Player is what the lookup found about one player.
type Player struct {
	Ranks [3]*Skill // 1s, 2s, 3s; nil if the player has none
	Wins  int       // -1 if not known
}

// Options configure New.
type Options struct {
	Token string                   // refresh token of the Epic account, "" if signed out
	Save  func(token, name string) // receives every new one: Epic replaces it with every sign-in

	Version     Version                                // found on GitHub before, if any
	SaveVersion func(Version)                          // receives a newer one found on GitHub
	Fetch       func(context.Context) (Version, error) // default FetchVersion
}

// Client keeps one connection to Psyonix's servers. Its methods may be
// called from several goroutines; they wait for each other.
type Client struct {
	opts Options

	egs      *rlapi.EGS
	signedIn atomic.Bool             // token != "", readable while a sign-in runs
	version  atomic.Pointer[Version] // of the game, readable likewise
	started  atomic.Bool             // the game started since the last look for its version
	mu       sync.Mutex
	token    string // refresh token of the Epic Games Launcher sign-in
	name     string
	rpc      *rlapi.PsyNetRPC
}

// New returns a client for a stored refresh token.
func New(opts Options) *Client {
	if opts.Fetch == nil {
		opts.Fetch = FetchVersion
	}
	if opts.SaveVersion == nil {
		opts.SaveVersion = func(Version) {}
	}
	c := &Client{opts: opts, egs: rlapi.NewEGS(), token: opts.Token}
	c.signedIn.Store(opts.Token != "")
	v := newest(opts.Version)
	c.version.Store(&v)
	return c
}

// keep stores a new refresh token; mu is held.
func (c *Client) keep(token, name string) {
	c.token, c.name = token, name
	c.signedIn.Store(token != "")
	c.opts.Save(token, name)
}

// LoginURL is Epic's sign-in page. Afterwards it shows a JSON page with
// the authorization code for Login.
func LoginURL() string { return rlapi.NewEGS().GetAuthURL() }

var codePattern = regexp.MustCompile(`"authorizationCode"\s*:\s*"([0-9a-fA-F]+)"`)

// parseCode takes the code out of the JSON page Epic shows, or a code
// pasted alone.
func parseCode(s string) string {
	if m := codePattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return strings.Trim(strings.TrimSpace(s), `"`)
}

// Login signs in with the authorization code from the page of LoginURL,
// pasted alone or as the whole page, and connects. It returns the Epic
// display name.
func (c *Client) Login(ctx context.Context, code string) (string, error) {
	code = parseCode(code)
	if code == "" {
		return "", errors.New("kein Code")
	}
	auth, err := c.egs.AuthenticateWithCode(code)
	if err != nil {
		return "", fmt.Errorf("Epic hat den Code nicht angenommen (er gilt nur kurz und einmal): %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	c.keep(auth.RefreshToken, auth.DisplayName)
	if err := c.connectLocked(ctx, true); err != nil {
		return c.name, err
	}
	return c.name, nil
}

// Logout closes the connection and forgets the account.
func (c *Client) Logout() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	c.keep("", "")
}

// SignedIn reports whether there is an account to connect with. It does
// not wait for a sign-in in progress.
func (c *Client) SignedIn() bool { return c.signedIn.Load() }

// GameStarted makes the next connection look on GitHub for a new game
// version first. The version only changes with an update of the game, and
// that comes before it starts.
func (c *Client) GameStarted() { c.started.Store(true) }

// Connect opens the connection ahead of the next lookup, unless it is
// still open.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked(ctx, true)
}

// Close ends the connection; the next lookup opens a new one.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *Client) closeLocked() {
	if c.rpc != nil {
		c.rpc.Close()
		c.rpc = nil
	}
}

// connectLocked signs in again with the refresh token and opens the
// WebSocket of PsyNet, unless it is still open. If the game started since
// the last look, it first looks on GitHub for a new game version, but only
// ahead of a lookup, not while one waits.
func (c *Client) connectLocked(ctx context.Context, ahead bool) error {
	if c.rpc != nil && c.rpc.IsConnected() {
		return nil
	}
	c.rpc = nil
	if c.token == "" {
		return ErrSignedOut
	}
	if ahead && c.started.Swap(false) {
		c.updateVersion(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	auth, err := c.egs.AuthenticateWithRefreshToken(c.token)
	if err != nil {
		// rlapi words a rejection by Epic like this, network errors differ
		if strings.HasPrefix(err.Error(), "authentication failed: 4") {
			c.keep("", c.name)
			return fmt.Errorf("%w: die Anmeldung ist abgelaufen, bitte neu anmelden", ErrSignedOut)
		}
		return fmt.Errorf("Epic: %w", err)
	}
	c.keep(auth.RefreshToken, auth.DisplayName)
	code, err := c.egs.GetExchangeCode(auth.AccessToken)
	if err != nil {
		return fmt.Errorf("Epic: %w", err)
	}
	eos, err := c.egs.ExchangeEOSToken(code)
	if err != nil {
		return fmt.Errorf("Epic Online Services: %w", err)
	}
	rpc, err := c.psyNet().AuthPlayer(eos.AccessToken, eos.AccountID, auth.DisplayName)
	if err != nil {
		return fmt.Errorf("Rocket League (Spielversion %s): %w", c.version.Load().Game, err)
	}
	c.rpc = rpc
	return nil
}

// Lookup returns ranks and wins of the players with the given Stats API
// IDs (Platform|ID|0). Players the servers do not know are missing.
func (c *Client) Lookup(ctx context.Context, ids []string) (map[string]Player, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for retry := false; ; retry = true {
		if err := c.connectLocked(ctx, false); err != nil {
			return nil, err
		}
		out, err := lookup(ctx, c.rpc, ids)
		if err == nil || retry || ctx.Err() != nil || c.rpc.IsConnected() {
			return out, err
		}
		// the connection broke, try once more with a new one
	}
}

func lookup(ctx context.Context, rpc *rlapi.PsyNetRPC, ids []string) (map[string]Player, error) {
	pids := make([]rlapi.PlayerID, len(ids))
	byKey := make(map[string]string, len(ids))
	for i, id := range ids {
		pids[i] = rlapi.PlayerID(id)
		byKey[key(id)] = id
	}
	// both questions go out at once, so the answer takes one round trip
	type winsResult struct {
		res *rlapi.GetStatLeaderboardRankForUsersResponse
		err error
	}
	winsCh := make(chan winsResult, 1)
	go func() {
		res, err := rpc.GetStatLeaderboardRankForUsers(ctx, "Wins", pids)
		winsCh <- winsResult{res, err}
	}()
	skills, err := rpc.GetPlayersSkills(ctx, pids)
	if err != nil {
		return nil, fmt.Errorf("Ränge: %w", err)
	}
	out := make(map[string]Player, len(ids))
	for _, p := range skills {
		id, ok := byKey[key(string(p.PlayerID))]
		if !ok {
			continue
		}
		found := Player{Wins: -1}
		for _, s := range p.Skills {
			if i := slices.Index(Playlists[:], rlapi.PlaylistID(s.Playlist)); i >= 0 {
				found.Ranks[i] = &Skill{Tier: s.Tier, Div: s.Division, MMR: s.MMR, Played: s.MatchesPlayed}
			}
		}
		out[id] = found
	}
	wins := <-winsCh
	if wins.err != nil {
		log.Printf("MMR-Abfrage: Siege nicht abrufbar: %v", wins.err) // the ranks are still worth showing
		return out, nil
	}
	for _, w := range wins.res.Players {
		if id, ok := byKey[key(string(w.PlayerID))]; ok {
			p := out[id]
			p.Wins = int(w.Value)
			out[id] = p
		}
	}
	return out, nil
}

// key compares player IDs without case and the trailing |0.
func key(id string) string {
	return strings.ToLower(strings.TrimSuffix(id, "|0"))
}
