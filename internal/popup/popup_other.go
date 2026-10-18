//go:build !windows && !linux && !darwin

package popup

import "log"

// Manager only logs on other systems.
type Manager struct{}

func New() *Manager { return &Manager{} }

func (m *Manager) Interactive() bool { return false }

func (m *Manager) ShowQuickSort(q Quick, pick func(int)) {
	log.Printf("Einordnen: %s → %s", q.Title, q.Target)
}

func (m *Manager) HideQuickSort() {}

func (m *Manager) Toast(t Toast) { log.Printf("%s: %s", t.Title, t.Body) }
