//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procShellExecuteEx      = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	procGetConsoleWindow    = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcesses = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole         = kernel32.NewProc("FreeConsole")
	procShowWindowConsole   = windows.NewLazySystemDLL("user32.dll").NewProc("ShowWindow")
)

const (
	seeMaskNoCloseProcess = 0x40
	seeMaskNoAsync        = 0x100
	swHide                = 0
)

// shellExecuteInfo mirrors SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       uintptr
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    uintptr
	idList     uintptr
	class      *uint16
	keyClass   uintptr
	hotKey     uint32
	icon       uintptr
	process    windows.Handle
}

// ErrCancelled means the user declined the administrator prompt.
var ErrCancelled = errors.New("abgebrochen")

// RunElevated runs this program again with args and administrator rights,
// after the Windows prompt, and waits for it. A non-zero exit code is an
// error.
func RunElevated(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		mask:       seeMaskNoCloseProcess | seeMaskNoAsync,
		verb:       windows.StringToUTF16Ptr("runas"),
		file:       windows.StringToUTF16Ptr(exe),
		parameters: windows.StringToUTF16Ptr(windows.ComposeCommandLine(args)),
		show:       windows.SW_HIDE,
	}
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		return err
	}
	if info.process == 0 {
		return errors.New("kein Prozess gestartet")
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("mit Administratorrechten fehlgeschlagen (Code %d)", code)
	}
	return nil
}

// HideConsole closes the console window Windows opens for a program built
// without -H windowsgui when it is started by double-click. A console the
// program shares with a terminal stays.
func HideConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	var pids [2]uint32
	if n, _, _ := procGetConsoleProcesses.Call(uintptr(unsafe.Pointer(&pids[0])), 2); n != 1 {
		return // started from a terminal
	}
	procShowWindowConsole.Call(hwnd, swHide)
	procFreeConsole.Call()
}
