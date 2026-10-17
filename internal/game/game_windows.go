//go:build windows

package game

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procSendInput   = user32.NewProc("SendInput")
	procMessageBeep = user32.NewProc("MessageBeep")
)

const (
	inputKeyboard        = 1
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfScancode    = 0x0008
	mbIconExclamation    = 0x30
)

// keyboardInput mirrors INPUT holding a KEYBDINPUT on 64-bit Windows.
type keyboardInput struct {
	typ       uint32
	_         uint32
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
	_         [8]byte // rest of the INPUT union
}

// sizeof(INPUT) is 40 on 64-bit Windows; these fail to compile otherwise.
var _ [40 - unsafe.Sizeof(keyboardInput{})]byte
var _ [unsafe.Sizeof(keyboardInput{}) - 40]byte

func sendKey(scan uint16, flags uint32) error {
	in := keyboardInput{typ: inputKeyboard, scan: scan, flags: keyeventfScancode | flags}
	n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if n != 1 {
		return fmt.Errorf("SendInput: %w", err)
	}
	return nil
}

// markReplay holds the game's "save replay" key (Backspace by default) for
// hold. It only sends input while Rocket League is the foreground window, so
// the key never lands in another program.
func markReplay(ctx context.Context, key string, hold time.Duration) error {
	sc, ok := keyCodes[key]
	if !ok {
		return fmt.Errorf("unbekannte Taste %q", key)
	}
	var flags uint32
	if sc.extended {
		flags = keyeventfExtendedKey
	}
	if !rocketLeagueFocused() {
		return ErrNotFocused
	}
	if err := sendKey(sc.code, flags); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
	if err := sendKey(sc.code, flags|keyeventfKeyUp); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !rocketLeagueFocused() {
		return ErrNotFocused // focus changed mid-hold, try again
	}
	return nil
}

// RocketLeagueRunning reports whether the game process exists.
func RocketLeagueRunning() bool {
	return findProcess(func(e *windows.ProcessEntry32) bool {
		return strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "RocketLeague.exe")
	})
}

func findProcess(match func(*windows.ProcessEntry32) bool) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if match(&e) {
			return true
		}
	}
	return false
}

// rocketLeagueFocused looks up the foreground window's executable through a
// process snapshot instead of opening a handle to the anti-cheat protected
// game process.
func rocketLeagueFocused() bool {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return false
	}
	return findProcess(func(e *windows.ProcessEntry32) bool {
		return e.ProcessID == pid && strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "RocketLeague.exe")
	})
}

func beep() {
	procMessageBeep.Call(mbIconExclamation)
}

// DefaultDemoDirs returns the replay folders of the Steam and Epic versions
// below the (possibly OneDrive-redirected) Documents folder.
func DefaultDemoDirs() ([]string, error) {
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(docs, "My Games", "Rocket League", "TAGame")
	return []string{filepath.Join(base, "Demos"), filepath.Join(base, "DemosEpic")}, nil
}
