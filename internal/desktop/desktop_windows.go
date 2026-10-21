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

// TrayHint says where to find the icon, SignIn when autostart runs the tool.
const (
	TrayHint = "Das Symbol findest du im Infobereich der Taskleiste, eventuell unter den ausgeblendeten Symbolen."
	SignIn   = "mit Windows"
)

// A left click on the icon opens the overlay.
const leftClickOpens = true

func trayHost() error { return nil }

// menuTitle shows the shortcut right-aligned after a tab.
func menuTitle(title, hotkey string) string {
	if hotkey == "" {
		return title
	}
	return title + "\t" + hotkey
}

var procSetProcessDpiAwarenessContext = windows.NewLazySystemDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")

// SetDPIAware keeps menus, message boxes and the overlay sharp on scaled
// displays. Call it before any window is created.
func SetDPIAware() {
	procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
}

// SingleInstance reports false if the tool already runs for this user with
// the same configuration file.
func SingleInstance(configPath string) bool {
	name := `Local\` + instanceKey(configPath)
	// Our handle stays open until the process exits. One to another
	// copy's mutex is closed, so the mutex goes away with that copy.
	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(name))
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(h)
		return false
	}
	return true
}

func ShowError(msg string) { messageBox(msg, mbIconError) }

func ShowInfo(msg string) { messageBox(msg, mbIconInformation) }

func messageBox(msg string, icon uint32) {
	windows.MessageBox(0, windows.StringToUTF16Ptr(msg), windows.StringToUTF16Ptr("roMod"), icon|mbSetForeground|mbTopmost)
}

// Edit opens a text file in Notepad.
func Edit(path string) error {
	return exec.Command("notepad.exe", path).Start()
}
