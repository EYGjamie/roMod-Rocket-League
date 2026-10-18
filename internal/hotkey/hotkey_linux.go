//go:build linux

package hotkey

import (
	"errors"
	"sync"

	"github.com/EYGjamie/auto-replay-upload/internal/x11"
	"github.com/jezek/xgb/xproto"
)

// Manager owns the system-wide hotkeys as key grabs on the X server. They
// work in X11 sessions and, under Wayland, while an X11 window such as the
// game (Proton runs it under XWayland) has the focus.
type Manager struct {
	c      *x11.Conn
	locks  uint16   // Caps Lock and Num Lock, ignored when matching
	ignore []uint16 // lock combinations every grab is repeated for

	mu       sync.Mutex
	grabs    map[string]grab
	released map[xproto.Keycode]xproto.Timestamp
}

type grab struct {
	code    xproto.Keycode
	mods    uint16
	pressed func()
}

// NewManager connects to the X server and starts reading its events.
func NewManager() (*Manager, error) {
	c, err := x11.Open()
	if err != nil {
		return nil, err
	}
	num := c.NumLockMask()
	m := &Manager{c: c, locks: xproto.ModMaskLock | num, grabs: map[string]grab{}, released: map[xproto.Keycode]xproto.Timestamp{}}
	m.ignore = []uint16{0, xproto.ModMaskLock}
	if num != 0 {
		m.ignore = append(m.ignore, num, num|xproto.ModMaskLock)
	}
	go m.loop()
	return m, nil
}

func (m *Manager) loop() {
	for {
		ev, err := m.c.X.WaitForEvent()
		if ev == nil && err == nil {
			return // connection closed
		}
		switch e := ev.(type) {
		case xproto.KeyPressEvent:
			m.mu.Lock()
			// Holding a key repeats it as release and press at the same time.
			repeat := m.released[e.Detail] == e.Time
			var fn func()
			for _, g := range m.grabs {
				if !repeat && g.code == e.Detail && g.mods == e.State&0xff&^m.locks {
					fn = g.pressed
				}
			}
			m.mu.Unlock()
			if fn != nil {
				go fn()
			}
		case xproto.KeyReleaseEvent:
			m.mu.Lock()
			m.released[e.Detail] = e.Time
			m.mu.Unlock()
		}
	}
}

// Set binds name to hk and calls pressed whenever it is hit. A name that is
// already bound is rebound; an empty hotkey only removes the binding.
func (m *Manager) Set(name string, hk Hotkey, pressed func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.grabs[name]; ok {
		m.ungrab(old, len(m.ignore))
		delete(m.grabs, name)
	}
	if hk.vk == 0 {
		return nil
	}
	code, ok := m.c.Keycode(keysym(hk.vk))
	if !ok {
		return errors.New("die Taste gibt es auf dieser Tastatur nicht")
	}
	g := grab{code: code, mods: xMods(hk.mods), pressed: pressed}
	for _, other := range m.grabs {
		if other.code == g.code && other.mods == g.mods {
			// X would hand out the same grab twice, and removing one would
			// remove both; RegisterHotKey refuses as well.
			return errors.New("schon für etwas anderes vergeben")
		}
	}
	for i, extra := range m.ignore {
		err := xproto.GrabKeyChecked(m.c.X, true, m.c.Root, g.mods|extra, code, xproto.GrabModeAsync, xproto.GrabModeAsync).Check()
		if err != nil {
			m.ungrab(g, i)
			if _, ok := err.(xproto.AccessError); ok {
				return errors.New("schon von einem anderen Programm belegt")
			}
			return err
		}
	}
	m.grabs[name] = g
	return nil
}

// ungrab releases the first n of g's grabs.
func (m *Manager) ungrab(g grab, n int) {
	for _, extra := range m.ignore[:n] {
		xproto.UngrabKey(m.c.X, g.code, m.c.Root, g.mods|extra)
	}
}

// Remove unbinds name.
func (m *Manager) Remove(name string) {
	m.Set(name, Hotkey{}, nil)
}

// keysym is the X symbol of a virtual-key code: lower-case letters, digits
// and F1–F24.
func keysym(vk uint32) xproto.Keysym {
	switch {
	case vk >= 'A' && vk <= 'Z':
		return xproto.Keysym(vk - 'A' + 'a')
	case vk >= 0x70 && vk <= 0x87:
		return xproto.Keysym(0xffbe + vk - 0x70)
	}
	return xproto.Keysym(vk)
}

// xMods turns RegisterHotKey's modifier flags into X modifier masks; Alt is
// Mod1 and the Windows (Super) key Mod4 on X11 keyboards.
func xMods(mods uint32) uint16 {
	var x uint16
	if mods&modAlt != 0 {
		x |= xproto.ModMask1
	}
	if mods&modCtrl != 0 {
		x |= xproto.ModMaskControl
	}
	if mods&modShift != 0 {
		x |= xproto.ModMaskShift
	}
	if mods&modWin != 0 {
		x |= xproto.ModMask4
	}
	return x
}
