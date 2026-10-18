package hotkey

import (
	"testing"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/x11"
	"github.com/EYGjamie/auto-replay-upload/internal/x11/x11test"
	"github.com/jezek/xgb/xproto"
)

func TestX11Hotkeys(t *testing.T) {
	x11test.Start(t)
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	defer m.c.Close()
	keyboard, err := x11.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer keyboard.Close()
	if !keyboard.HasXTest() {
		t.Skip("XTEST fehlt")
	}
	code := func(sym xproto.Keysym) xproto.Keycode {
		c, ok := keyboard.Keycode(sym)
		if !ok {
			t.Fatalf("no key for %#x", sym)
		}
		return c
	}
	ctrl, alt, shift, numLock := code(0xffe3), code(0xffe9), code(0xffe1), code(0xff7f)
	tap := func(keys ...xproto.Keycode) {
		for _, k := range keys {
			keyboard.FakeKey(k, true)
		}
		for i := len(keys) - 1; i >= 0; i-- {
			keyboard.FakeKey(keys[i], false)
		}
	}
	hits := make(chan string, 8)
	expect := func(want string) {
		t.Helper()
		wait := 3 * time.Second
		if want == "" {
			wait = 500 * time.Millisecond
		}
		select {
		case got := <-hits:
			if got != want {
				t.Errorf("hotkey %s fired, want %s", got, want)
			}
		case <-time.After(wait):
			if want != "" {
				t.Errorf("hotkey %s did not fire", want)
			}
			return
		}
		if want == "" {
			t.Error("a hotkey fired, want none")
		}
	}

	overlay, _ := Parse("Ctrl+Alt+B")
	if err := m.Set("overlay", overlay, func() { hits <- "overlay" }); err != nil {
		t.Fatal(err)
	}
	pause, _ := Parse("Strg+Shift+F9") // Ctrl+Alt+F1–F12 switch the console on Linux
	if err := m.Set("pause", pause, func() { hits <- "pause" }); err != nil {
		t.Fatal(err)
	}
	tap(ctrl, alt, code('b'))
	expect("overlay")
	tap(ctrl, shift, code(0xffc6)) // F9
	expect("pause")
	tap(ctrl, code('b')) // without Alt
	tap(ctrl, alt, code('n'))

	tap(numLock) // works with Num Lock on, too
	tap(ctrl, alt, code('b'))
	expect("overlay")
	tap(numLock)

	m.Remove("overlay")
	tap(ctrl, alt, code('b'))
	expect("")

	// a combination another program grabbed first
	other, _ := x11.Open()
	defer other.Close()
	xproto.GrabKeyChecked(other.X, true, other.Root, xproto.ModMaskControl|xproto.ModMask1, code('k'), xproto.GrabModeAsync, xproto.GrabModeAsync).Check()
	taken, _ := Parse("Ctrl+Alt+K")
	if err := m.Set("taken", taken, func() {}); err == nil || err.Error() != "schon von einem anderen Programm belegt" {
		t.Errorf("Set(taken combination) = %v", err)
	}
}
