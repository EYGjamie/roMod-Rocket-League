//go:build windows

package hotkey

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey = user32.NewProc("RegisterHotKey")
	procGetMessage     = user32.NewProc("GetMessageW")
)

const (
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
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

// Start registers a system-wide hotkey and calls pressed whenever it is hit,
// also while a fullscreen game has focus.
func Start(hk Hotkey, pressed func()) error {
	errc := make(chan error, 1)
	go func() {
		// WM_HOTKEY goes to the registering thread's message queue.
		runtime.LockOSThread()
		if r, _, err := procRegisterHotKey.Call(0, 1, uintptr(hk.mods|modNoRepeat), uintptr(hk.vk)); r == 0 {
			errc <- err
			return
		}
		errc <- nil
		var msg winMsg
		for {
			if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0); int32(r) <= 0 {
				return
			}
			if msg.message == wmHotkey {
				go pressed()
			}
		}
	}()
	return <-errc
}
