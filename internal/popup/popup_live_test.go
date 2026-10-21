//go:build windows

package popup

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindowEx     = user32.NewProc("FindWindowExW")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
)

// TestLiveWindows shows real popups for a moment (set POPUP_LIVE=1): they
// must exist, stay on top, never take focus and go away again.
func TestLiveWindows(t *testing.T) {
	if os.Getenv("POPUP_LIVE") == "" {
		t.Skip("POPUP_LIVE=1 zeigt kurz echte Fenster")
	}
	m := New()
	before := windows.GetForegroundWindow()
	m.ShowQuickSort(Quick{Title: "Test · DFH Stadium · Sieg 3:1", Target: "2s Ranked", Options: [4]string{"Scrims", "", "", "Eingang"}, Seconds: 5}, func(int) {})
	m.Toast(Toast{Title: "Replay hochgeladen", Body: "Testbenachrichtigung", Actions: []Action{{Label: "OK"}}})
	time.Sleep(800 * time.Millisecond)

	class := windows.StringToUTF16Ptr("RoModPopup")
	pid := windows.GetCurrentProcessId()
	var found []uintptr
	for hwnd := uintptr(0); ; {
		hwnd, _, _ = procFindWindowEx.Call(0, hwnd, uintptr(unsafe.Pointer(class)), 0)
		if hwnd == 0 {
			break
		}
		var p uint32
		windows.GetWindowThreadProcessId(windows.HWND(hwnd), &p)
		if p == pid {
			found = append(found, hwnd)
		}
	}
	if len(found) != 2 {
		t.Fatalf("%d popup windows, want 2", len(found))
	}
	for _, hwnd := range found {
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE
		var r rect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if ex&wsExTopmost == 0 || ex&wsExNoActivate == 0 || visible == 0 || r.right-r.left < 200 || r.bottom-r.top < 60 {
			t.Errorf("window %x: exstyle %x, visible %d, rect %+v", hwnd, ex, visible, r)
		}
	}
	if fg := windows.GetForegroundWindow(); fg != before {
		for _, hwnd := range found {
			if uintptr(fg) == hwnd {
				t.Error("a popup took the focus")
			}
		}
	}

	m.HideQuickSort()
	done := make(chan struct{})
	m.do(func() {
		for len(m.toasts) > 0 {
			m.closeWindow(m.toasts[0])
		}
		close(done)
	})
	<-done
	time.Sleep(200 * time.Millisecond)
	for _, hwnd := range found {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible != 0 {
			t.Errorf("window %x still visible", hwnd)
		}
	}
}
