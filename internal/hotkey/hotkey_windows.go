//go:build windows

package hotkey

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessage        = user32.NewProc("GetMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
)

const (
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	wmApp       = 0x8000
)

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// Manager owns the system-wide hotkeys. They fire even while a fullscreen
// game has focus. RegisterHotKey binds a hotkey to the calling thread, so
// all registrations run on one locked thread with its own message loop.
type Manager struct {
	thread uint32
	reqs   chan func()

	mu       sync.Mutex
	ids      map[string]int
	handlers map[int]func()
	nextID   int
}

// NewManager starts the hotkey thread.
func NewManager() (*Manager, error) {
	m := &Manager{reqs: make(chan func(), 16), ids: map[string]int{}, handlers: map[int]func(){}, nextID: 1}
	ready := make(chan struct{})
	go m.loop(ready)
	<-ready
	return m, nil
}

func (m *Manager) loop(ready chan struct{}) {
	runtime.LockOSThread()
	m.thread = windows.GetCurrentThreadId()
	close(ready)
	var msg winMsg
	for {
		if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		switch msg.message {
		case wmApp:
			for drained := false; !drained; {
				select {
				case f := <-m.reqs:
					f()
				default:
					drained = true
				}
			}
		case wmHotkey:
			m.mu.Lock()
			fn := m.handlers[int(msg.wParam)]
			m.mu.Unlock()
			if fn != nil {
				go fn()
			}
		}
	}
}

// run executes f on the hotkey thread and returns its error.
func (m *Manager) run(f func() error) error {
	done := make(chan error, 1)
	m.reqs <- func() { done <- f() }
	procPostThreadMessage.Call(uintptr(m.thread), wmApp, 0, 0)
	return <-done
}

// Set binds name to hk and calls pressed whenever it is hit. A name that is
// already bound is rebound; an empty hotkey only removes the binding.
func (m *Manager) Set(name string, hk Hotkey, pressed func()) error {
	return m.run(func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if id, ok := m.ids[name]; ok {
			procUnregisterHotKey.Call(0, uintptr(id))
			delete(m.ids, name)
			delete(m.handlers, id)
		}
		if hk.vk == 0 {
			return nil
		}
		id := m.nextID
		m.nextID++
		if r, _, err := procRegisterHotKey.Call(0, uintptr(id), uintptr(hk.mods|modNoRepeat), uintptr(hk.vk)); r == 0 {
			if errors.Is(err, windows.ERROR_HOTKEY_ALREADY_REGISTERED) {
				return errors.New("schon von einem anderen Programm belegt")
			}
			return err
		}
		m.ids[name] = id
		m.handlers[id] = pressed
		return nil
	})
}

// Remove unbinds name.
func (m *Manager) Remove(name string) {
	m.Set(name, Hotkey{}, nil)
}
