//go:build linux

package game

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/EYGjamie/romod/internal/x11"
	"github.com/jezek/xgb/xproto"
)

// DefaultDemoDirs returns the replay folders found in the Proton prefix of
// every Steam library (also Flatpak and Snap) and in the Wine prefixes of
// Heroic, Lutris and Bottles.
func DefaultDemoDirs() ([]string, error) { return searchDemoDirs(linuxPatterns) }

// GameDirs returns where the game may be installed: Steam libraries,
// Heroic (also as Flatpak) and Legendary, and the Wine prefixes.
func GameDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return installDirs(home, linuxPatterns,
		filepath.Join(home, ".config", "heroic", "legendaryConfig", "legendary", "installed.json"),
		filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "legendaryConfig", "legendary", "installed.json"),
		filepath.Join(home, ".config", "legendary", "installed.json"))
}

// RocketLeagueRunning reports whether the game process exists.
func RocketLeagueRunning() bool { return procRunning("/proc") }

// The save key goes through the X server, so it reaches the game under X11
// and under XWayland alike.

var display struct {
	sync.Mutex
	conn *x11.Conn
}

// xconn returns the X connection, opened on first use and again after it
// broke.
func xconn() (*x11.Conn, error) {
	display.Lock()
	defer display.Unlock()
	if display.conn == nil {
		c, err := x11.Open()
		if err != nil {
			return nil, err
		}
		display.conn = c
	}
	return display.conn, nil
}

func dropConn(c *x11.Conn) {
	display.Lock()
	defer display.Unlock()
	if c != nil && display.conn == c {
		c.Close()
		display.conn = nil
	}
}

// SaveKeySupported reports whether keys can be sent: an X server with the
// XTEST extension.
func SaveKeySupported() bool {
	c, err := xconn()
	return err == nil && c.HasXTest()
}

// markReplay holds the game's "save replay" key (Backspace by default) for
// hold. It only sends input while Rocket League is the active window, so
// the key never lands in another program.
func markReplay(ctx context.Context, key string, hold time.Duration) error {
	sc, ok := keyCodes[key]
	if !ok {
		return fmt.Errorf("unbekannte Taste %q", key)
	}
	c, err := xconn()
	if err != nil {
		return fmt.Errorf("Tastendruck braucht X11: %w", err)
	}
	if !rocketLeagueFocused(c) {
		return ErrNotFocused
	}
	code := xKeycode(sc)
	if err := c.FakeKey(code, true); err != nil {
		dropConn(c)
		return err
	}
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
	if err := c.FakeKey(code, false); err != nil {
		dropConn(c)
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !rocketLeagueFocused(c) {
		return ErrNotFocused // focus changed mid-hold, try again
	}
	return nil
}

// rocketLeagueFocused checks the active window's WM_CLASS, which Wine sets
// to the .exe and Proton to steam_app_252950, and its process.
func rocketLeagueFocused(c *x11.Conn) bool {
	w, ok, err := c.ActiveWindow()
	if err != nil {
		dropConn(c)
		return false
	}
	if !ok {
		return false
	}
	for _, class := range w.Class {
		if isRocketLeague(class) || class == "steam_app_"+rocketLeagueAppID {
			return true
		}
	}
	return w.PID != 0 && isGameProcess("/proc", strconv.Itoa(int(w.PID)))
}

// extendedKeys maps the keys with the E0 prefix to Linux input codes. For
// the others the input code is the scan code.
var extendedKeys = map[uint16]uint16{
	0x1D: 97, 0x38: 100, // right Ctrl, AltGr
	0x52: 110, 0x53: 111, 0x47: 102, 0x4F: 107, 0x49: 104, 0x51: 109, // Insert, Delete, Home, End, PageUp, PageDown
	0x48: 103, 0x50: 108, 0x4B: 105, 0x4D: 106, // arrows
	0x35: 98, 0x1C: 96, // numpad / and Enter
}

// xKeycode is the key code X uses for a physical key: the Linux input code
// plus 8.
func xKeycode(sc scanCode) xproto.Keycode {
	code := sc.code
	if sc.extended {
		code = extendedKeys[sc.code]
	}
	return xproto.Keycode(code + 8)
}

// beep plays the desktop's warning sound, or rings the X bell.
func beep() {
	go func() {
		const sound = "/usr/share/sounds/freedesktop/stereo/dialog-warning.oga"
		if exec.Command("canberra-gtk-play", "--id=dialog-warning").Run() == nil {
			return
		}
		if _, err := os.Stat(sound); err == nil {
			for _, player := range []string{"paplay", "pw-play"} {
				if exec.Command(player, sound).Run() == nil {
					return
				}
			}
		}
		if c, err := xconn(); err == nil {
			c.Bell()
		}
	}()
}
