//go:build !windows

package desktop

import "errors"

// ErrCancelled means the user declined the administrator prompt.
var ErrCancelled = errors.New("abgebrochen")

// RunElevated is only offered on Windows, where the game's folder needs
// administrator rights. Elsewhere it lies in the user's home.
func RunElevated(args ...string) error { return errors.ErrUnsupported }

// HideConsole only matters on Windows.
func HideConsole() {}
