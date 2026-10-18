//go:build darwin

package popup

import (
	"log"
	"os/exec"
)

// Manager shows notifications in the Notification Center through
// osascript. macOS offers no buttons for them, so actions and the
// quick-sort prompt are left out: rules sort right away.
type Manager struct{}

func New() *Manager { return &Manager{} }

// Interactive is false: the quick-sort prompt cannot take a choice here.
func (m *Manager) Interactive() bool { return false }

func (m *Manager) ShowQuickSort(q Quick, pick func(int)) {
	log.Printf("Einordnen: %s → %s", q.Title, q.Target)
}

func (m *Manager) HideQuickSort() {}

func (m *Manager) Toast(t Toast) {
	go exec.Command("osascript", "-e", `on run argv
	display notification (item 2 of argv) with title (item 1 of argv)
end run`, t.Title, t.Body).Run()
}
