//go:build !windows

package game

import (
	"context"
	"errors"
	"time"
)

// Marking replays needs Windows APIs, and there is no default replay folder
// elsewhere; uploading still works with demo_dirs set.

func markReplay(context.Context, time.Duration) error {
	return errors.New("Tastendruck wird nur unter Windows unterstützt")
}

func beep() {}

func DefaultDemoDirs() ([]string, error) {
	return nil, errors.New("kein Standardordner außerhalb von Windows, bitte demo_dirs setzen")
}
