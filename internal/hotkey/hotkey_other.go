//go:build !windows

package hotkey

import "errors"

func Start(Hotkey, func()) error {
	return errors.New("Hotkeys werden nur unter Windows unterstützt")
}
