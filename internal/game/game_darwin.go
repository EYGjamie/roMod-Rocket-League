//go:build darwin

package game

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// DefaultDemoDirs returns the replay folders found in the bottles of
// CrossOver and Whisky and in Heroic's prefixes.
func DefaultDemoDirs() ([]string, error) { return searchDemoDirs(macPatterns) }

// GameDirs returns where the game may be installed: the bottles of
// CrossOver and Whisky, Heroic and Legendary.
func GameDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return installDirs(home, macPatterns,
		filepath.Join(home, "Library", "Application Support", "heroic", "legendaryConfig", "legendary", "installed.json"),
		filepath.Join(home, ".config", "legendary", "installed.json"))
}

// On a Mac the game only runs under Wine, and macOS lets no program type
// into another without the accessibility permission, so the tool does not
// press the save key here. The beep at the end of a match still reminds
// to save the replay.
func SaveKeySupported() bool { return false }

func markReplay(context.Context, string, time.Duration) error {
	return errors.New("Tastendruck wird auf dem Mac nicht unterstützt")
}

func beep() { go exec.Command("afplay", "/System/Library/Sounds/Funk.aiff").Run() }

// RocketLeagueRunning looks through the user's processes for the .exe that
// Wine runs.
func RocketLeagueRunning() bool {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return false
	}
	for _, p := range procs {
		if isRocketLeague(unix.ByteSliceToString(p.Proc.P_comm[:])) {
			return true
		}
		raw, err := unix.SysctlRaw("kern.procargs2", int(p.Proc.P_pid))
		if err != nil {
			continue
		}
		exe, args := procArgs(raw)
		if isRocketLeague(exe) {
			return true
		}
		for _, a := range args {
			if isRocketLeague(a) {
				return true
			}
		}
	}
	return false
}
