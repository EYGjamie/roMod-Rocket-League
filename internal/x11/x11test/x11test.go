//go:build linux

// Package x11test runs a private X server for tests.
package x11test

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Start points $DISPLAY at a fresh Xvfb that stops with the test, so tests
// never send keys to the real desktop. Without Xvfb the test is skipped.
func Start(t testing.TB) {
	t.Helper()
	path, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb fehlt")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	cmd := exec.Command(path, "-displayfd", "3", "-screen", "0", "640x480x24", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{w} // fd 3: the server writes its display number when ready
	err = cmd.Start()
	w.Close()
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	display := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(r).ReadString('\n')
		display <- strings.TrimSpace(line)
	}()
	select {
	case d := <-display:
		if d == "" {
			t.Skip("Xvfb startet nicht")
		}
		t.Setenv("DISPLAY", ":"+d)
	case <-time.After(10 * time.Second):
		t.Skip("Xvfb startet nicht")
	}
}

// Window opens its own connection and a window with WM_CLASS class
// ("instance\x00class\x00") and _NET_WM_PID pid, focused, which receives
// key events on the returned connection.
func Window(t testing.TB, class string, pid uint32) (*xgb.Conn, xproto.Window) {
	t.Helper()
	x, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(x.Close)
	screen := xproto.Setup(x).DefaultScreen(x)
	win, err := xproto.NewWindowId(x)
	if err != nil {
		t.Fatal(err)
	}
	err = xproto.CreateWindowChecked(x, screen.RootDepth, win, screen.Root, 0, 0, 100, 100, 0, xproto.WindowClassInputOutput,
		screen.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease}).Check()
	if err != nil {
		t.Fatal(err)
	}
	atom, err := xproto.InternAtom(x, false, uint16(len("_NET_WM_PID")), "_NET_WM_PID").Reply()
	if err != nil {
		t.Fatal(err)
	}
	v := make([]byte, 4)
	xgb.Put32(v, pid)
	xproto.ChangeProperty(x, xproto.PropModeReplace, win, atom.Atom, xproto.AtomCardinal, 32, 1, v)
	xproto.ChangeProperty(x, xproto.PropModeReplace, win, xproto.AtomWmClass, xproto.AtomString, 8, uint32(len(class)), []byte(class))
	if err := xproto.MapWindowChecked(x, win).Check(); err != nil {
		t.Fatal(err)
	}
	if err := xproto.SetInputFocusChecked(x, xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	return x, win
}

// Keys collects the key presses ("+22") and releases ("-22") arriving on x
// until the test ends.
func Keys(t testing.TB, x *xgb.Conn) func(n int, wait time.Duration) []string {
	ch := make(chan string, 64)
	go func() {
		for {
			ev, err := x.WaitForEvent()
			if ev == nil && err == nil {
				return // closed
			}
			switch e := ev.(type) {
			case xproto.KeyPressEvent:
				ch <- "+" + strconv.Itoa(int(e.Detail))
			case xproto.KeyReleaseEvent:
				ch <- "-" + strconv.Itoa(int(e.Detail))
			}
		}
	}()
	return func(n int, wait time.Duration) []string {
		var got []string
		deadline := time.After(wait)
		for len(got) < n {
			select {
			case k := <-ch:
				got = append(got, k)
			case <-deadline:
				return got
			}
		}
		return got
	}
}
