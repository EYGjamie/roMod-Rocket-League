//go:build linux

package x11

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

func init() {
	xgb.Logger = log.New(io.Discard, "", 0) // errors reach the callers anyway
}

// Conn is a connection to the X server. It is safe for concurrent use.
type Conn struct {
	X     *xgb.Conn
	Root  xproto.Window
	setup *xproto.SetupInfo

	mu    sync.Mutex
	atoms map[string]xproto.Atom
	xtest error // result of setting up XTEST, once tried
	tried bool
}

// Open connects to the display in $DISPLAY.
func Open() (*Conn, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, errors.New("keine X11-Anzeige (DISPLAY ist leer)")
	}
	x, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	setup := xproto.Setup(x)
	return &Conn{X: x, Root: setup.DefaultScreen(x).Root, setup: setup, atoms: map[string]xproto.Atom{}}, nil
}

func (c *Conn) Close() { c.X.Close() }

// atom returns the atom called name, 0 while the server does not know it
// (no program has used the name yet).
func (c *Conn) atom(name string) xproto.Atom {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.atoms[name]; ok {
		return a
	}
	r, err := xproto.InternAtom(c.X, true, uint16(len(name)), name).Reply()
	if err != nil || r.Atom == 0 {
		return 0
	}
	c.atoms[name] = r.Atom
	return r.Atom
}

// property returns the value of a property of win; empty if unset.
func (c *Conn) property(win xproto.Window, prop, typ xproto.Atom) ([]byte, error) {
	if prop == 0 {
		return nil, nil
	}
	r, err := xproto.GetProperty(c.X, false, win, prop, typ, 0, 1024).Reply()
	if err != nil {
		return nil, err
	}
	return r.Value, nil
}

// Window describes a top-level window.
type Window struct {
	PID   uint32   // process that created it, 0 if unknown
	Class []string // WM_CLASS: instance and class name
}

// ActiveWindow returns the window that has the focus. ok is false if no X
// window has it, for example while a Wayland window is in front.
func (c *Conn) ActiveWindow() (w Window, ok bool, err error) {
	v, err := c.property(c.Root, c.atom("_NET_ACTIVE_WINDOW"), xproto.AtomWindow)
	if err != nil {
		return w, false, err
	}
	var win xproto.Window
	if len(v) >= 4 {
		win = xproto.Window(xgb.Get32(v)) // 0 = none
	} else { // no window manager hints: ask the server
		r, err := xproto.GetInputFocus(c.X).Reply()
		if err != nil {
			return w, false, err
		}
		win = r.Focus
	}
	// The focus may sit on a child of the top-level window.
	for win > 1 && win != c.Root { // 0 = None, 1 = PointerRoot
		if w, err = c.describe(win); err != nil {
			return w, false, err
		}
		if w.PID != 0 || len(w.Class) > 0 {
			return w, true, nil
		}
		r, err := xproto.QueryTree(c.X, win).Reply()
		if err != nil {
			return w, false, err
		}
		win = r.Parent
	}
	return Window{}, false, nil
}

func (c *Conn) describe(win xproto.Window) (Window, error) {
	var w Window
	v, err := c.property(win, c.atom("_NET_WM_PID"), xproto.AtomCardinal)
	if err != nil {
		return w, err
	}
	if len(v) >= 4 {
		w.PID = xgb.Get32(v)
	}
	v, err = c.property(win, xproto.AtomWmClass, xproto.AtomString)
	if err != nil {
		return w, err
	}
	for _, s := range strings.Split(string(v), "\x00") {
		if s != "" {
			w.Class = append(w.Class, s)
		}
	}
	return w, nil
}

// Keycode returns a key that produces sym in any group or level.
func (c *Conn) Keycode(sym xproto.Keysym) (xproto.Keycode, bool) {
	first, last := c.setup.MinKeycode, c.setup.MaxKeycode
	r, err := xproto.GetKeyboardMapping(c.X, first, byte(last-first+1)).Reply()
	if err != nil || r.KeysymsPerKeycode == 0 {
		return 0, false
	}
	per := int(r.KeysymsPerKeycode)
	for i, s := range r.Keysyms {
		if s == sym {
			return first + xproto.Keycode(i/per), true
		}
	}
	return 0, false
}

// Keysym returns the first symbol of a key, for checks.
func (c *Conn) Keysym(code xproto.Keycode) (xproto.Keysym, bool) {
	r, err := xproto.GetKeyboardMapping(c.X, code, 1).Reply()
	if err != nil || len(r.Keysyms) == 0 {
		return 0, false
	}
	return r.Keysyms[0], true
}

const keysymNumLock = 0xff7f

// NumLockMask returns the modifier bit of Num Lock (usually Mod2), which
// hotkey grabs have to ignore along with Caps Lock.
func (c *Conn) NumLockMask() uint16 {
	key, ok := c.Keycode(keysymNumLock)
	if !ok {
		return 0
	}
	r, err := xproto.GetModifierMapping(c.X).Reply()
	if err != nil || r.KeycodesPerModifier == 0 {
		return xproto.ModMask2
	}
	for i, k := range r.Keycodes {
		if k == key {
			return 1 << (i / int(r.KeycodesPerModifier))
		}
	}
	return 0
}

// FakeKey presses or releases a key through the XTEST extension, the same
// way the keyboard would. The key goes to the window with the focus.
func (c *Conn) FakeKey(code xproto.Keycode, down bool) error {
	if err := c.initXTest(); err != nil {
		return err
	}
	typ := byte(xproto.KeyRelease)
	if down {
		typ = xproto.KeyPress
	}
	return xtest.FakeInputChecked(c.X, typ, byte(code), 0, c.Root, 0, 0, 0).Check()
}

// HasXTest reports whether keys can be simulated.
func (c *Conn) HasXTest() bool { return c.initXTest() == nil }

func (c *Conn) initXTest() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.tried {
		c.tried = true
		if err := xtest.Init(c.X); err != nil {
			c.xtest = errors.New("die X-Erweiterung XTEST fehlt")
		}
	}
	return c.xtest
}

// Bell rings the X server's bell.
func (c *Conn) Bell() { xproto.Bell(c.X, 0) }
