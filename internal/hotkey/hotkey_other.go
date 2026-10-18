//go:build !windows && !linux

package hotkey

import (
	"errors"
	"runtime"
)

// Manager does nothing here: on macOS system-wide hotkeys would need the
// accessibility permission and native code.
type Manager struct{}

func NewManager() (*Manager, error) {
	if runtime.GOOS == "darwin" {
		return nil, errors.New("auf dem Mac nicht unterstützt")
	}
	return nil, errors.New("auf diesem System nicht unterstützt")
}

func (m *Manager) Set(name string, hk Hotkey, pressed func()) error {
	if hk.vk == 0 {
		return nil
	}
	return errors.New("Hotkeys werden auf diesem System nicht unterstützt")
}

func (m *Manager) Remove(string) {}
