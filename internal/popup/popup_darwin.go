//go:build darwin

package popup

import (
	"log"
	"os/exec"
	"sync"
)

// Manager shows notifications in the Notification Center through
// osascript. macOS offers no buttons for them, so actions and the
// quick-sort prompt are left out: rules sort right away.
type Manager struct {
	mu    sync.Mutex
	lobby string // text of the last lobby notification
}

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

// ShowLobby shows the lobby as a notification, unless the same one was
// shown last: notifications cannot be updated in place.
func (m *Manager) ShowLobby(l Lobby) {
	text := lobbyText(l)
	m.mu.Lock()
	same := text == m.lobby
	m.lobby = text
	m.mu.Unlock()
	if !same {
		m.Toast(Toast{Title: l.Title, Body: text})
	}
}

// HideLobby forgets the last lobby, so showing it again notifies again.
func (m *Manager) HideLobby() {
	m.mu.Lock()
	m.lobby = ""
	m.mu.Unlock()
}
