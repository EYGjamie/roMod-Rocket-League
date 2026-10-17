//go:build !windows

package hotkey

import "errors"

// Manager does nothing outside Windows.
type Manager struct{}

func NewManager() (*Manager, error) { return &Manager{}, nil }

func (m *Manager) Set(name string, hk Hotkey, pressed func()) error {
	if hk.vk == 0 {
		return nil
	}
	return errors.New("Hotkeys werden nur unter Windows unterstützt")
}

func (m *Manager) Remove(string) {}
