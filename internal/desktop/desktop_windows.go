//go:build windows

package desktop

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

const (
	mbIconError       = 0x10
	mbIconInformation = 0x40
	mbSetForeground   = 0x10000
	mbTopmost         = 0x40000

	dpiAwarenessPerMonitorV2 = ^uintptr(3) // -4 as two's complement
)

var procSetProcessDpiAwarenessContext = windows.NewLazySystemDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")

// SetDPIAware keeps menus, message boxes and the overlay sharp on scaled
// displays. Call it before any window is created.
func SetDPIAware() {
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
}

// SingleInstance reports false if the tool is already running for this user.
func SingleInstance() bool {
	// The handle stays open until the process exits.
	_, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\auto-replay-upload`))
	return err != windows.ERROR_ALREADY_EXISTS
}

func ShowError(msg string) { messageBox(msg, mbIconError) }

func ShowInfo(msg string) { messageBox(msg, mbIconInformation) }

func messageBox(msg string, icon uint32) {
	windows.MessageBox(0, windows.StringToUTF16Ptr(msg), windows.StringToUTF16Ptr("auto-replay-upload"), icon|mbSetForeground|mbTopmost)
}

// Edit opens a text file in Notepad.
func Edit(path string) error {
	return exec.Command("notepad.exe", path).Start()
}
