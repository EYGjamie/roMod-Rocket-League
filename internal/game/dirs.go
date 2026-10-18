//go:build linux || darwin

package game

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Outside Windows, Rocket League runs under Proton or Wine, which keep the
// Windows folders in a prefix. The replay folders are searched there.

// rocketLeagueAppID is the game's Steam app; Proton keeps a prefix per app.
const rocketLeagueAppID = "252950"

// tagameInPrefix is the game's folder in a Wine prefix, as a glob pattern.
const tagameInPrefix = "drive_c/users/*/Documents/My Games/Rocket League/TAGame"

// linuxPatterns lists where the game's TAGame folder can be: the Proton
// prefix in every Steam library, the Wine prefixes of Heroic (Epic),
// Lutris, Bottles and plain Wine, and the old native version.
func linuxPatterns(home string) []string {
	var p []string
	for _, lib := range steamLibraries(home) {
		p = append(p, filepath.Join(globEscape(lib), "steamapps", "compatdata", rocketLeagueAppID, "pfx", tagameInPrefix))
	}
	h := globEscape(home)
	return append(p,
		filepath.Join(h, "Games", "Heroic", "Prefixes", "*", tagameInPrefix),
		filepath.Join(h, "Games", "Heroic", "Prefixes", "*", "*", tagameInPrefix),
		filepath.Join(h, "Games", "*", tagameInPrefix),
		filepath.Join(h, ".local", "share", "bottles", "bottles", "*", tagameInPrefix),
		filepath.Join(h, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles", "*", tagameInPrefix),
		filepath.Join(h, ".wine", tagameInPrefix),
		filepath.Join(h, ".local", "share", "Rocket League", "TAGame"),
	)
}

// macPatterns lists the bottles of CrossOver and Whisky, Heroic's prefixes,
// plain Wine and the old native version.
func macPatterns(home string) []string {
	h := globEscape(home)
	return []string{
		filepath.Join(h, "Library", "Application Support", "CrossOver", "Bottles", "*", tagameInPrefix),
		filepath.Join(h, "Library", "Containers", "com.isaacmarovitz.Whisky", "Bottles", "*", tagameInPrefix),
		filepath.Join(h, "Games", "Heroic", "Prefixes", "*", tagameInPrefix),
		filepath.Join(h, "Games", "Heroic", "Prefixes", "*", "*", tagameInPrefix),
		filepath.Join(h, ".wine", tagameInPrefix),
		filepath.Join(h, "Library", "Application Support", "Rocket League", "TAGame"),
	}
}

// steamRoots are where Steam may be installed: natively (with its links),
// from Debian's package, as Flatpak or as Snap.
func steamRoots(home string) []string {
	return []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "debian-installation"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
	}
}

var vdfPath = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

// steamLibraries returns the Steam installations and the further library
// folders listed in their libraryfolders.vdf, each once.
func steamLibraries(home string) []string {
	unescape := strings.NewReplacer(`\\`, `\`, `\"`, `"`)
	var libs []string
	for _, root := range steamRoots(home) {
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		libs = append(libs, root)
		for _, vdf := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
			data, _ := os.ReadFile(vdf)
			for _, m := range vdfPath.FindAllStringSubmatch(string(data), -1) {
				libs = append(libs, unescape.Replace(m[1]))
			}
		}
	}
	return unique(libs)
}

// findDemoDirs expands patterns of TAGame folders and returns the replay
// folders in them that exist (Demos, and DemosEpic of the Epic version).
func findDemoDirs(patterns []string) []string {
	var dirs []string
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, tagame := range matches {
			for _, name := range []string{"Demos", "DemosEpic"} {
				dir := filepath.Join(tagame, name)
				if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
					dirs = append(dirs, dir)
				}
			}
		}
	}
	return unique(dirs)
}

// unique drops paths that lead to the same place as an earlier one.
func unique(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		key := filepath.Clean(p)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			key = real
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

// globEscape quotes the pattern characters in a fixed part of a pattern.
func globEscape(path string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(path)
}

var found struct {
	sync.Mutex
	at   time.Time
	dirs []string
}

// searchDemoDirs searches at most every 30 seconds; the upload queue asks
// every few seconds and a new prefix only appears with the game's first
// start.
func searchDemoDirs(patterns func(home string) []string) ([]string, error) {
	found.Lock()
	defer found.Unlock()
	if found.at.IsZero() || time.Since(found.at) > 30*time.Second {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		found.dirs, found.at = findDemoDirs(patterns(home)), time.Now()
	}
	if len(found.dirs) == 0 {
		return nil, errors.New("kein Replay-Ordner gefunden, bitte in den Einstellungen hinzufügen")
	}
	return slices.Clone(found.dirs), nil
}
