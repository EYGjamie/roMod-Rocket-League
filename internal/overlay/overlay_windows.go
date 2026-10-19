//go:build windows

package overlay

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtr      = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSendMessage           = user32.NewProc("SendMessageW")
	procPostMessage           = user32.NewProc("PostMessageW")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

const (
	wsPopup                     = 0x80000000
	wsExToolWindow              = 0x00000080 // no taskbar button
	swHide                      = 0
	swpNoSize                   = 0x0001
	swpNoMove                   = 0x0002
	swpFrameChanged             = 0x0020
	swpShowWindow               = 0x0040
	wmSize                      = 0x0005
	wmNCLButtonDown             = 0x00A1
	htCaption                   = 2
	monitorDefaultToNull        = 0
	monitorDefaultToPrimary     = 1
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2

	// negative handles and indices as two's complement
	hwndTopmost = ^uintptr(0)  // -1
	gwlStyle    = ^uintptr(15) // -16
	gwlExStyle  = ^uintptr(19) // -20
)

type rect struct{ left, top, right, bottom int32 }

type monitorInfo struct {
	size    uint32
	monitor rect
	work    rect
	flags   uint32
}

// Native reports that the overlay is an own window over the game.
const Native = true

// Overlay is a frameless, always-on-top window showing the GUI page. It is
// created on first use and afterwards only hidden and shown again.
type Overlay struct {
	url      string
	mu       sync.Mutex
	view     webview2.WebView
	hwnd     uintptr
	creating bool
	route    string // view to open once the window exists
	moved    bool   // dragged by the user; only used on the window's thread
}

func NewOverlay(url string) *Overlay {
	return &Overlay{url: url}
}

// Toggle opens the overlay, brings it to the front, or hides it when it
// already is in front.
func (o *Overlay) Toggle() {
	o.mu.Lock()
	if o.view != nil && windows.IsWindowVisible(windows.HWND(o.hwnd)) && uintptr(windows.GetForegroundWindow()) == o.hwnd {
		hwnd := o.hwnd
		o.view.Dispatch(func() { procShowWindow.Call(hwnd, swHide) })
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	o.Show("")
}

// Show opens the overlay in front, at a view of the interface such as
// "eingang" if route is not empty.
func (o *Overlay) Show(route string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	switch {
	case o.creating:
		if route != "" {
			o.route = route
		}
	case o.view == nil:
		o.creating, o.route = true, route
		go o.run()
	default:
		view, hwnd := o.view, o.hwnd
		view.Dispatch(func() {
			if route != "" {
				view.Eval("location.hash = " + strconv.Quote("#/"+route))
			}
			view.Eval("window.onOverlayShown && window.onOverlayShown()")
			showOverlay(hwnd, o.moved)
		})
	}
}

// run creates the window and runs its message loop on a dedicated thread,
// as WebView2 requires.
func (o *Overlay) run() {
	runtime.LockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)

	_, _, w, h := overlayBounds()
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  webviewDataPath(),
		WindowOptions: webview2.WindowOptions{
			Title:  "auto-replay-upload",
			Width:  uint(w),
			Height: uint(h),
			Center: true,
		},
	})
	if view == nil {
		o.mu.Lock()
		o.creating = false
		o.mu.Unlock()
		log.Print("WebView2 ist nicht verfügbar, öffne die Seite im Browser")
		if err := OpenBrowser(o.url); err != nil {
			log.Printf("Browser konnte nicht geöffnet werden: %v", err)
		}
		return
	}

	hwnd := uintptr(view.Window())
	procShowWindow.Call(hwnd, swHide)
	procSetWindowLongPtr.Call(hwnd, gwlStyle, wsPopup)
	procSetWindowLongPtr.Call(hwnd, gwlExStyle, wsExToolWindow)
	corners := uint32(dwmwcpRound) // Windows 11 only, ignored elsewhere
	procDwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corners)), 4)
	view.Bind("closeOverlay", func() { procShowWindow.Call(hwnd, swHide) })
	o.moved = false
	view.Bind("dragOverlay", func() {
		// Hand the pressed mouse button over to the move loop of Windows, as
		// if the click had hit a title bar. Posted, so the loop runs after
		// WebView2 has finished delivering this call.
		o.moved = true
		procReleaseCapture.Call()
		procPostMessage.Call(hwnd, wmNCLButtonDown, htCaption, 0)
	})
	o.mu.Lock()
	url := o.url
	if o.route != "" {
		url += "#/" + o.route
	}
	o.mu.Unlock()
	view.Navigate(url)
	showOverlay(hwnd, false)

	o.mu.Lock()
	o.view, o.hwnd, o.creating = view, hwnd, false
	o.mu.Unlock()

	view.Run() // returns once the window was destroyed

	o.mu.Lock()
	o.view, o.hwnd = nil, 0
	o.mu.Unlock()
}

// showOverlay puts the window on top and focuses it. It centers the window
// on the monitor of the current foreground window (usually the game), unless
// keep is set and the window is still on a monitor.
func showOverlay(hwnd uintptr, keep bool) {
	var x, y, w, h int32
	flags := uintptr(swpFrameChanged | swpShowWindow)
	if mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNull); keep && mon != 0 {
		flags |= swpNoMove | swpNoSize
	} else {
		x, y, w, h = overlayBounds()
	}
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
	procSendMessage.Call(hwnd, wmSize, 0, 0) // let WebView2 fill the new client area
	procSetForegroundWindow.Call(hwnd)
}

func overlayBounds() (x, y, w, h int32) {
	work := rect{0, 0, 1920, 1080}
	mon, _, _ := procMonitorFromWindow.Call(uintptr(windows.GetForegroundWindow()), monitorDefaultToPrimary)
	mi := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
		work = mi.work
	}
	mw, mh := work.right-work.left, work.bottom-work.top
	w, h = mw*7/10, mh*8/10
	return work.left + (mw-w)/2, work.top + (mh-h)/2, w, h
}

func webviewDataPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "" // library default
	}
	return filepath.Join(dir, "auto-replay-upload", "WebView2")
}

// OpenBrowser opens a URL in the default browser.
func OpenBrowser(url string) error {
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(url), nil, nil, windows.SW_SHOWNORMAL)
}
