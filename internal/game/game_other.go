//go:build !windows && !linux && !darwin

package game

import (
	"context"
	"errors"
	"time"
)

// Elsewhere there is no default replay folder and no key press; uploading
// still works with demo_dirs set.

func SaveKeySupported() bool { return false }

func markReplay(context.Context, string, time.Duration) error {
	return errors.New("Tastendruck wird auf diesem System nicht unterstützt")
}

func beep() {}

func DefaultDemoDirs() ([]string, error) {
	return nil, errors.New("kein Standardordner auf diesem System, bitte demo_dirs setzen")
}

func RocketLeagueRunning() bool { return false }

func GameDirs() []string { return nil }
