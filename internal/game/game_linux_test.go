package game

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/EYGjamie/auto-replay-upload/internal/x11"
	"github.com/EYGjamie/auto-replay-upload/internal/x11/x11test"
	"github.com/jezek/xgb/xproto"
)

func useXvfb(t *testing.T) *x11.Conn {
	t.Helper()
	x11test.Start(t)
	dropConn(display.conn) // a connection to an earlier test's server
	t.Cleanup(func() { dropConn(display.conn) })
	c, err := xconn()
	if err != nil {
		t.Fatal(err)
	}
	if !SaveKeySupported() {
		t.Skip("XTEST fehlt")
	}
	return c
}

// The X key codes have to match what the keyboard sends for the physical
// key, as the browser names it; checked against the US layout.
func TestXKeycodes(t *testing.T) {
	c := useXvfb(t)
	want := map[string]xproto.Keysym{
		"Backspace": 0xff08, "Enter": 0xff0d, "Space": 0x20, "Tab": 0xff09, "KeyQ": 'q', "KeyA": 'a', "KeyM": 'm',
		"Digit1": '1', "Digit0": '0', "Minus": '-', "Slash": '/', "IntlBackslash": '<', "F1": 0xffbe, "F10": 0xffc7,
		"F11": 0xffc8, "F12": 0xffc9, "ControlRight": 0xffe4, "Insert": 0xff63, "Delete": 0xffff, "Home": 0xff50,
		"End": 0xff57, "PageUp": 0xff55, "PageDown": 0xff56, "ArrowUp": 0xff52, "ArrowDown": 0xff54,
		"ArrowLeft": 0xff51, "ArrowRight": 0xff53, "Numpad7": 0xff95, "NumpadEnter": 0xff8d, "NumpadDivide": 0xffaf,
		"NumpadMultiply": 0xffaa, "NumpadSubtract": 0xffad, "NumpadAdd": 0xffab,
	}
	for name, sym := range want {
		got, ok := c.Keysym(xKeycode(keyCodes[name]))
		if !ok || got != sym {
			t.Errorf("%s: key %d gives keysym %#x, want %#x", name, xKeycode(keyCodes[name]), got, sym)
		}
	}
	for name, sc := range keyCodes {
		if code := xKeycode(sc); code <= 8 {
			t.Errorf("%s has no X key code", name)
		}
	}
}

func TestMarkReplayOnlyInTheGame(t *testing.T) {
	useXvfb(t)
	ctx := context.Background()

	x, _ := x11test.Window(t, "kate\x00kate\x00", 1)
	keys := x11test.Keys(t, x)
	if err := markReplay(ctx, "Backspace", 50*time.Millisecond); !errors.Is(err, ErrNotFocused) {
		t.Fatalf("markReplay in another window = %v, want ErrNotFocused", err)
	}
	if got := keys(1, 300*time.Millisecond); len(got) != 0 {
		t.Fatalf("keys reached another window: %q", got)
	}

	x, _ = x11test.Window(t, "steam_app_252950\x00steam_app_252950\x00", 2)
	keys = x11test.Keys(t, x)
	if err := markReplay(ctx, "Backspace", 50*time.Millisecond); err != nil {
		t.Fatalf("markReplay in the game = %v", err)
	}
	if got := keys(2, 3*time.Second); !slices.Equal(got, []string{"+22", "-22"}) {
		t.Errorf("game got %q, want Backspace pressed and released", got)
	}
}
