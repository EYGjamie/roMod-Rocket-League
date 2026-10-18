package x11

import (
	"fmt"
	"testing"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/x11/x11test"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// intern creates an atom, as a window manager would.
func intern(t *testing.T, c *Conn, name string) xproto.Atom {
	t.Helper()
	r, err := xproto.InternAtom(c.X, false, uint16(len(name)), name).Reply()
	if err != nil {
		t.Fatal(err)
	}
	return r.Atom
}

// window creates a mapped window with the given process ID and WM_CLASS.
func window(t *testing.T, c *Conn, pid uint32, class string) xproto.Window {
	t.Helper()
	id, err := xproto.NewWindowId(c.X)
	if err != nil {
		t.Fatal(err)
	}
	screen := xproto.Setup(c.X).DefaultScreen(c.X)
	err = xproto.CreateWindowChecked(c.X, screen.RootDepth, id, c.Root, 0, 0, 100, 100, 0,
		xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease}).Check()
	if err != nil {
		t.Fatal(err)
	}
	pidBytes := make([]byte, 4)
	xgb.Put32(pidBytes, pid)
	xproto.ChangeProperty(c.X, xproto.PropModeReplace, id, intern(t, c, "_NET_WM_PID"), xproto.AtomCardinal, 32, 1, pidBytes)
	xproto.ChangeProperty(c.X, xproto.PropModeReplace, id, xproto.AtomWmClass, xproto.AtomString, 8, uint32(len(class)), []byte(class))
	if err := xproto.MapWindowChecked(c.X, id).Check(); err != nil {
		t.Fatal(err)
	}
	return id
}

func focus(t *testing.T, c *Conn, win xproto.Window) {
	t.Helper()
	if err := xproto.SetInputFocusChecked(c.X, xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestActiveWindow(t *testing.T) {
	x11test.Start(t)
	c, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok, err := c.ActiveWindow(); ok || err != nil {
		t.Fatalf("ActiveWindow() without a focused window = %v, %v", ok, err)
	}
	game := window(t, c, 4242, "rocketleague.exe\x00steam_app_252950\x00")
	other := window(t, c, 77, "kate\x00kate\x00")

	focus(t, c, game) // no window manager: the focus counts
	w, ok, err := c.ActiveWindow()
	if err != nil || !ok || w.PID != 4242 || len(w.Class) != 2 || w.Class[1] != "steam_app_252950" {
		t.Fatalf("ActiveWindow() = %+v, %v, %v", w, ok, err)
	}

	// with a window manager its _NET_ACTIVE_WINDOW wins over the focus
	setActive := func(win xproto.Window) {
		v := make([]byte, 4)
		xgb.Put32(v, uint32(win))
		xproto.ChangePropertyChecked(c.X, xproto.PropModeReplace, c.Root, intern(t, c, "_NET_ACTIVE_WINDOW"), xproto.AtomWindow, 32, 1, v).Check()
	}
	setActive(other)
	if w, ok, _ := c.ActiveWindow(); !ok || w.PID != 77 {
		t.Errorf("ActiveWindow() with _NET_ACTIVE_WINDOW = %+v, %v, want PID 77", w, ok)
	}
	setActive(0) // a Wayland window is in front
	if w, ok, _ := c.ActiveWindow(); ok {
		t.Errorf("ActiveWindow() with no active window = %+v, want none", w)
	}
}

func TestFakeKeyReachesFocusedWindow(t *testing.T) {
	x11test.Start(t)
	c, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !c.HasXTest() {
		t.Skip("XTEST fehlt")
	}
	win := window(t, c, 1, "test\x00test\x00")
	focus(t, c, win)
	backspace, ok := c.Keycode(0xff08)
	if !ok || backspace != 22 {
		t.Fatalf("Keycode(BackSpace) = %d, %v; want the evdev code 22", backspace, ok)
	}
	if err := c.FakeKey(backspace, true); err != nil {
		t.Fatal(err)
	}
	if err := c.FakeKey(backspace, false); err != nil {
		t.Fatal(err)
	}
	got := make(chan xgb.Event, 4)
	go func() {
		for {
			ev, err := c.X.WaitForEvent()
			if ev == nil && err == nil {
				return
			}
			if ev != nil {
				got <- ev
			}
		}
	}()
	var keys []string
	for len(keys) < 2 {
		select {
		case ev := <-got:
			switch e := ev.(type) { // the server may send others, such as MappingNotify
			case xproto.KeyPressEvent:
				keys = append(keys, fmt.Sprintf("press %d", e.Detail))
			case xproto.KeyReleaseEvent:
				keys = append(keys, fmt.Sprintf("release %d", e.Detail))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("key events %q, want a press and a release", keys)
		}
	}
	if keys[0] != "press 22" || keys[1] != "release 22" {
		t.Errorf("key events %q, want press and release of 22", keys)
	}
}

func TestNumLockMask(t *testing.T) {
	x11test.Start(t)
	c, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if m := c.NumLockMask(); m != xproto.ModMask2 {
		t.Errorf("NumLockMask() = %#x, want Mod2", m)
	}
}
