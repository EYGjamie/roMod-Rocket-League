//go:build windows

package popup

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")
	msimg  = windows.NewLazySystemDLL("msimg32.dll")

	procRegisterClassEx       = user32.NewProc("RegisterClassExW")
	procCreateWindowEx        = user32.NewProc("CreateWindowExW")
	procDefWindowProc         = user32.NewProc("DefWindowProcW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetMessage            = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessage       = user32.NewProc("DispatchMessageW")
	procPostThreadMessage     = user32.NewProc("PostThreadMessageW")
	procBeginPaint            = user32.NewProc("BeginPaint")
	procEndPaint              = user32.NewProc("EndPaint")
	procInvalidateRect        = user32.NewProc("InvalidateRect")
	procSetTimer              = user32.NewProc("SetTimer")
	procGetDC                 = user32.NewProc("GetDC")
	procReleaseDC             = user32.NewProc("ReleaseDC")
	procFillRect              = user32.NewProc("FillRect")
	procDrawText              = user32.NewProc("DrawTextW")
	procLoadCursor            = user32.NewProc("LoadCursorW")
	procSetCursor             = user32.NewProc("SetCursor")
	procMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	procTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	procSetWindowRgn          = user32.NewProc("SetWindowRgn")
	procCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp   = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateDIBSection      = gdi32.NewProc("CreateDIBSection")
	procSelectObject          = gdi32.NewProc("SelectObject")
	procDeleteObject          = gdi32.NewProc("DeleteObject")
	procDeleteDC              = gdi32.NewProc("DeleteDC")
	procBitBlt                = gdi32.NewProc("BitBlt")
	procCreateFont            = gdi32.NewProc("CreateFontW")
	procSetTextColor          = gdi32.NewProc("SetTextColor")
	procSetBkMode             = gdi32.NewProc("SetBkMode")
	procCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	procCreatePen             = gdi32.NewProc("CreatePen")
	procRoundRect             = gdi32.NewProc("RoundRect")
	procGetStockObject        = gdi32.NewProc("GetStockObject")
	procGetTextMetrics        = gdi32.NewProc("GetTextMetricsW")
	procSetTextCharExtra      = gdi32.NewProc("SetTextCharacterExtra")
	procCreateRectRgn         = gdi32.NewProc("CreateRectRgn")
	procCreateRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	procCombineRgn            = gdi32.NewProc("CombineRgn")
	procAlphaBlend            = msimg.NewProc("AlphaBlend")
	procGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	swShowNoActive  = 4
	swpNoActivate   = 0x0010
	swpShowWindow   = 0x0040
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmEraseBkgnd    = 0x0014
	wmSetCursor     = 0x0020
	wmMouseActivate = 0x0021
	wmTimer         = 0x0113
	wmMouseMove     = 0x0200
	wmLButtonUp     = 0x0202
	wmMouseLeave    = 0x02A3
	wmApp           = 0x8000
	maNoActivate    = 3
	tmeLeave        = 0x2
	idcArrow        = 32512
	idcHand         = 32649
	dtCenter        = 0x1
	dtRight         = 0x2
	dtVCenter       = 0x4
	dtWordBreak     = 0x10
	dtSingleLine    = 0x20
	dtCalcRect      = 0x400
	dtNoPrefix      = 0x800
	dtEndEllipsis   = 0x8000
	srcCopy         = 0x00CC0020
	monitorPrimary  = 1
	hwndTopmost     = ^uintptr(0) // -1
	psSolid         = 0
	psDash          = 1
	nullBrush       = 5
	rgnOr           = 2
	blendAlpha      = 0x01ff0000 // AC_SRC_OVER with the source's own alpha (AC_SRC_ALPHA)

	timerQuick = 1
	timerToast = 2
	timerLobby = 3

	toastLife = 8 * time.Second
	maxToasts = 3
)

type rect struct{ left, top, right, bottom int32 }

func (r rect) contains(x, y int32) bool {
	return x >= r.left && x < r.right && y >= r.top && y < r.bottom
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type paintStruct struct {
	hdc       uintptr
	erase     int32
	paint     rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

type monitorInfo struct {
	size    uint32
	monitor rect
	work    rect
	flags   uint32
}

type trackMouse struct {
	size      uint32
	flags     uint32
	hwnd      uintptr
	hoverTime uint32
}

type bitmapInfoHeader struct {
	size                   uint32
	width, height          int32
	planes, bitCount       uint16
	compression, sizeImage uint32
	xPels, yPels           int32
	clrUsed, clrImportant  uint32
}

// colors as 0x00BBGGRR
const (
	colBg      = 0x00281f1b
	colBorder  = 0x00483e36
	colText    = 0x00ede9e6
	colMuted   = 0x00afa49a
	colAccent  = 0x00ff8d4c
	colChip    = 0x00383028
	colChipHot = 0x00544a40
	colTrack   = 0x00383028
	colOnAccnt = 0x00100d0b
)

// The lobby has the colors of the interface (Nocturne).
const (
	lobBg      = 0x00261816 // #161826
	lobSurface = 0x00322523 // #232532
	lobText    = 0x00ede9e9 // #e9e9ed
	lobKey     = 0x00f5e7e4 // #e4e7f5, neutral 200
	lobNote    = 0x00cab6b2 // #b2b6ca, neutral 400
	lobMuted   = 0x00ab9793 // #9397ab, neutral 500
	lobEdge    = 0x006c5d59 // #595d6c, neutral 700
	lobTrack   = 0x00312b29 // #292b31, neutral 900
	lobAccent  = 0x00d98491 // #9184d9
	lobMe      = 0x00fee5e7 // #e7e5fe, accent 200
	lobBlue    = 0x00fdced2 // #d2cefd, accent 300
	lobTagBg   = 0x006a3a42 // #423a6a, accent 800
	lobMeBg    = 0x0041272b // #2b2741, accent 900
	lobTag     = 0x00fff4f5 // #f5f4ff, accent 100
	lobOrange  = 0x0069a8e8 // #e8a869, oklch(0.78 0.11 65)
)

// hit is a clickable area of a popup.
type hit struct {
	r   rect
	run func()
}

// window is one popup.
type window struct {
	hwnd    uintptr
	quick   *Quick
	pick    func(int)
	started time.Time
	toast   *Toast
	lobby   *Lobby
	shown   time.Time
	hover   bool
	hot     int
	dpi     uint32
	w, h    int32
	hits    []hit
	parts   []rect // rounded parts the window is cut to, nil for all of it
	close   func() // closes this popup
}

// Manager owns the popup windows. They live on one locked thread with
// its own message loop.
type Manager struct {
	thread  uint32
	reqs    chan func()
	class   *uint16
	wins    map[uintptr]*window
	quick   *window
	lobby   *window
	toasts  []*window
	pending []Toast
	fonts   map[[2]int32]uintptr // by height and weight
	logos   map[[2]int32]uintptr // bitmaps by tier and size, 0 if missing
}

var (
	mgrOnce sync.Once
	mgr     *Manager
)

// New starts the popup thread. There is only one manager per process.
func New() *Manager {
	mgrOnce.Do(func() {
		mgr = &Manager{reqs: make(chan func(), 32), wins: map[uintptr]*window{}, fonts: map[[2]int32]uintptr{}, logos: map[[2]int32]uintptr{}}
		ready := make(chan struct{})
		go mgr.loop(ready)
		<-ready
	})
	return mgr
}

func (m *Manager) loop(ready chan struct{}) {
	runtime.LockOSThread()
	m.thread = windows.GetCurrentThreadId()
	m.class = windows.StringToUTF16Ptr("RoModPopup")
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{wndProc: windows.NewCallback(wndProc), cursor: cursor, className: m.class}
	wc.size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	close(ready)
	var ms msg
	for {
		if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&ms)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		if ms.message == wmApp && ms.hwnd == 0 {
			for drained := false; !drained; {
				select {
				case f := <-m.reqs:
					f()
				default:
					drained = true
				}
			}
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&ms)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&ms)))
	}
}

func (m *Manager) do(f func()) {
	m.reqs <- f
	procPostThreadMessage.Call(uintptr(m.thread), wmApp, 0, 0)
}

// Interactive reports that the quick-sort prompt takes a choice.
func (m *Manager) Interactive() bool { return true }

// ShowQuickSort shows the prompt; pick receives 1–4 for a click on an
// option.
func (m *Manager) ShowQuickSort(q Quick, pick func(int)) {
	m.do(func() {
		if m.quick != nil {
			m.destroy(m.quick)
		}
		w := &window{quick: &q, pick: pick, started: time.Now(), hot: -1}
		m.quick = w
		m.open(w)
		procSetTimer.Call(w.hwnd, timerQuick, 100, 0)
	})
}

// HideQuickSort removes the prompt.
func (m *Manager) HideQuickSort() {
	m.do(func() {
		if m.quick != nil {
			m.destroy(m.quick)
			m.quick = nil
		}
	})
}

// ShowLobby shows the lobby at the top in the middle, like the scoreboard
// of the game, or updates the one shown. Its countdown starts again.
func (m *Manager) ShowLobby(l Lobby) {
	m.do(func() {
		if w := m.lobby; w != nil && w.hwnd != 0 {
			w.lobby, w.shown = &l, time.Now()
			w.w, w.h = m.measure(w)
			shape(w)
			m.place()
			procInvalidateRect.Call(w.hwnd, 0, 0)
			return
		}
		w := &window{lobby: &l, shown: time.Now(), hot: -1}
		m.lobby = w
		m.open(w)
		procSetTimer.Call(w.hwnd, timerLobby, 40, 0)
	})
}

// HideLobby removes the lobby popup.
func (m *Manager) HideLobby() {
	m.do(func() {
		if m.lobby != nil {
			m.destroy(m.lobby)
			m.lobby = nil
		}
	})
}

// Toast shows a notification at the bottom right, at most three at once.
func (m *Manager) Toast(t Toast) {
	m.do(func() {
		if len(m.toasts) >= maxToasts {
			m.pending = append(m.pending, t)
			return
		}
		m.showToast(t)
	})
}

func (m *Manager) showToast(t Toast) {
	w := &window{toast: &t, shown: time.Now(), hot: -1}
	m.toasts = append(m.toasts, w)
	m.open(w)
	procSetTimer.Call(w.hwnd, timerToast, 500, 0)
	m.place()
}

// workArea returns the work area and DPI of the monitor with the
// foreground window, usually the game.
func workArea() (rect, uint32) {
	mon, _, _ := procMonitorFromWindow.Call(uintptr(windows.GetForegroundWindow()), monitorPrimary)
	mi := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	work := rect{0, 0, 1920, 1080}
	if ok, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
		work = mi.work
	}
	var dx, dy uint32 = 96, 96
	if procGetDpiForMonitor.Find() == nil {
		procGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy)))
	}
	return work, dx
}

func (m *Manager) open(w *window) {
	_, dpi := workArea()
	w.dpi = dpi
	w.w, w.h = m.measure(w)
	w.close = func() { m.closeWindow(w) }
	hwnd, _, _ := procCreateWindowEx.Call(wsExTopmost|wsExToolWindow|wsExNoActivate, uintptr(unsafe.Pointer(m.class)),
		0, wsPopup, 0, 0, uintptr(w.w), uintptr(w.h), 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	w.hwnd = hwnd
	m.wins[hwnd] = w
	if w.parts == nil {
		corners := uint32(2) // DWMWCP_ROUND, Windows 11 only
		procDwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&corners)), 4)
	}
	shape(w)
	m.place()
	procShowWindow.Call(hwnd, swShowNoActive)
}

// shape cuts the window to its parts, the game shows between them.
func shape(w *window) {
	if w.parts == nil || w.hwnd == 0 {
		return
	}
	d := uintptr(scale(16, w.dpi)) // corner diameter, as the parts are drawn
	rgn, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
	for _, p := range w.parts {
		// one more at the right and bottom: the region leaves out what RoundRect draws there
		part, _, _ := procCreateRoundRectRgn.Call(uintptr(p.left), uintptr(p.top), uintptr(p.right+1), uintptr(p.bottom+1), d, d)
		procCombineRgn.Call(rgn, rgn, part, rgnOr)
		procDeleteObject.Call(part)
	}
	procSetWindowRgn.Call(w.hwnd, rgn, 1) // the window owns rgn now
}

// place positions the prompt at the top right, the lobby at the top in the
// middle and stacks notifications from the bottom right upwards.
func (m *Manager) place() {
	work, dpi := workArea()
	margin := scale(16, dpi)
	if w := m.quick; w != nil && w.hwnd != 0 {
		procSetWindowPos.Call(w.hwnd, hwndTopmost, uintptr(work.right-w.w-margin), uintptr(work.top+margin),
			uintptr(w.w), uintptr(w.h), swpNoActivate|swpShowWindow)
	}
	if w := m.lobby; w != nil && w.hwnd != 0 {
		procSetWindowPos.Call(w.hwnd, hwndTopmost, uintptr(work.left+(work.right-work.left-w.w)/2), uintptr(work.top+margin),
			uintptr(w.w), uintptr(w.h), swpNoActivate|swpShowWindow)
	}
	y := work.bottom - margin
	for i := len(m.toasts) - 1; i >= 0; i-- {
		w := m.toasts[i]
		if w.hwnd == 0 {
			continue
		}
		y -= w.h
		procSetWindowPos.Call(w.hwnd, hwndTopmost, uintptr(work.right-w.w-margin), uintptr(y),
			uintptr(w.w), uintptr(w.h), swpNoActivate|swpShowWindow)
		y -= scale(10, dpi)
	}
}

func (m *Manager) destroy(w *window) {
	if w.hwnd != 0 {
		delete(m.wins, w.hwnd)
		procDestroyWindow.Call(w.hwnd)
		w.hwnd = 0
	}
}

func (m *Manager) closeWindow(w *window) {
	m.destroy(w)
	if w == m.quick {
		m.quick = nil
		return
	}
	if w == m.lobby {
		m.lobby = nil
		return
	}
	for i, t := range m.toasts {
		if t == w {
			m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
			break
		}
	}
	if len(m.pending) > 0 && len(m.toasts) < maxToasts {
		next := m.pending[0]
		m.pending = m.pending[1:]
		m.showToast(next)
	}
	m.place()
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	m := mgr
	w := m.wins[hwnd]
	if w == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	switch message {
	case wmMouseActivate:
		return maNoActivate
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		m.paintBuffered(hdc, w)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmTimer:
		if wParam == timerToast && !w.hover && time.Since(w.shown) > toastLife {
			m.closeWindow(w)
			return 0
		}
		if wParam == timerQuick || wParam == timerLobby {
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmMouseMove:
		x, y := int32(int16(lParam)), int32(int16(lParam>>16))
		if !w.hover {
			tme := trackMouse{flags: tmeLeave, hwnd: hwnd}
			tme.size = uint32(unsafe.Sizeof(tme))
			procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			w.hover = true
		}
		hot := -1
		for i, h := range w.hits {
			if h.r.contains(x, y) {
				hot = i
			}
		}
		if hot != w.hot {
			w.hot = hot
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmMouseLeave:
		w.hover, w.hot = false, -1
		if w.toast != nil && time.Since(w.shown) > toastLife-3*time.Second {
			w.shown = time.Now().Add(-toastLife + 3*time.Second) // three more seconds after leaving
		}
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmSetCursor:
		if w.hot >= 0 {
			c, _, _ := procLoadCursor.Call(0, idcHand)
			procSetCursor.Call(c)
			return 1
		}
	case wmLButtonUp:
		if w.hot >= 0 && w.hot < len(w.hits) {
			if run := w.hits[w.hot].run; run != nil {
				run()
			}
		}
		return 0
	case wmDestroy:
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// paintBuffered draws into a memory bitmap first to avoid flicker.
func (m *Manager) paintBuffered(hdc uintptr, w *window) {
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	bmp, _, _ := procCreateCompatibleBmp.Call(hdc, uintptr(w.w), uintptr(w.h))
	old, _, _ := procSelectObject.Call(mem, bmp)
	m.paint(mem, w, time.Now())
	procBitBlt.Call(hdc, 0, 0, uintptr(w.w), uintptr(w.h), mem, 0, 0, srcCopy)
	procSelectObject.Call(mem, old)
	procDeleteObject.Call(bmp)
	procDeleteDC.Call(mem)
}

func scale(v int32, dpi uint32) int32 { return v * int32(dpi) / 96 }

// font returns a cached Segoe UI font of pt points.
func (m *Manager) font(pt10 int32, bold bool, dpi uint32) uintptr {
	weight := int32(400)
	if bold {
		weight = 600
	}
	return m.fontPx(pt10*int32(dpi)/720, weight) // pt10 is tenths of a point
}

// fontPx returns a cached Segoe UI font whose letters are px pixels high,
// as font-size in CSS.
func (m *Manager) fontPx(px, weight int32) uintptr {
	key := [2]int32{px, weight}
	if f, ok := m.fonts[key]; ok {
		return f
	}
	f, _, _ := procCreateFont.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Segoe UI"))))
	m.fonts[key] = f
	return f
}

// text draws s in r and returns the rectangle it needs (with dtCalcRect).
func (m *Manager) text(hdc uintptr, s string, r rect, pt10 int32, bold bool, color uint32, flags uintptr, dpi uint32) rect {
	return drawText(hdc, s, r, m.font(pt10, bold, dpi), color, flags)
}

func drawText(hdc uintptr, s string, r rect, font uintptr, color uint32, flags uintptr) rect {
	procSelectObject.Call(hdc, font)
	procSetTextColor.Call(hdc, uintptr(color))
	procSetBkMode.Call(hdc, 1) // transparent
	p, _ := windows.UTF16PtrFromString(s)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|dtNoPrefix)
	return r
}

// ascent returns how far the letters of font reach above the baseline,
// and the height of a line.
func ascent(hdc, font uintptr) (above, line int32) {
	var tm [16]int32 // TEXTMETRICW, tmHeight and tmAscent first
	procSelectObject.Call(hdc, font)
	procGetTextMetrics.Call(hdc, uintptr(unsafe.Pointer(&tm)))
	return tm[1], tm[0]
}

func fill(hdc uintptr, r rect, color uint32) {
	brush, _, _ := procCreateSolidBrush.Call(uintptr(color))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
	procDeleteObject.Call(brush)
}

func roundRect(hdc uintptr, r rect, radius int32, fillColor, border uint32) {
	brush, _, _ := procCreateSolidBrush.Call(uintptr(fillColor))
	pen, _, _ := procCreatePen.Call(0, 1, uintptr(border))
	ob, _, _ := procSelectObject.Call(hdc, brush)
	op, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(radius), uintptr(radius))
	procSelectObject.Call(hdc, ob)
	procSelectObject.Call(hdc, op)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

// outline draws only the edge of a rounded rectangle, solid or dashed.
func outline(hdc uintptr, r rect, radius int32, style uintptr, color uint32) {
	brush, _, _ := procGetStockObject.Call(nullBrush)
	pen, _, _ := procCreatePen.Call(style, 1, uintptr(color))
	ob, _, _ := procSelectObject.Call(hdc, brush)
	op, _, _ := procSelectObject.Call(hdc, pen)
	procSetBkMode.Call(hdc, 1) // gaps of a dashed pen stay clear
	procRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(radius), uintptr(radius))
	procSelectObject.Call(hdc, ob)
	procSelectObject.Call(hdc, op)
	procDeleteObject.Call(pen)
}

// drawLogo draws the logo of a tier into the square r; false if there is
// none.
func (m *Manager) drawLogo(hdc uintptr, tier int, r rect) bool {
	size := r.right - r.left
	key := [2]int32{int32(tier), size}
	bmp, ok := m.logos[key]
	if !ok {
		if px, err := rankLogo(tier, int(size)); err == nil {
			hdr := bitmapInfoHeader{width: size, height: -size, planes: 1, bitCount: 32} // top-down
			hdr.size = uint32(unsafe.Sizeof(hdr))
			var bits unsafe.Pointer
			bmp, _, _ = procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&hdr)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
			if bmp != 0 {
				copy(unsafe.Slice((*byte)(bits), len(px)), px)
			}
		}
		m.logos[key] = bmp
	}
	if bmp == 0 {
		return false
	}
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	old, _, _ := procSelectObject.Call(mem, bmp)
	procAlphaBlend.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(size), uintptr(size), mem, 0, 0, uintptr(size), uintptr(size), blendAlpha)
	procSelectObject.Call(mem, old)
	procDeleteDC.Call(mem)
	return true
}

// measure computes the popup size by laying it out on a screen DC.
func (m *Manager) measure(w *window) (int32, int32) {
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)
	width := scale(360, w.dpi)
	if w.lobby != nil {
		width = scale(lobbyW, w.dpi)
	}
	w.w, w.h = width, 2000
	h := m.layout(hdc, w, time.Now(), false)
	return width, h
}

// paint draws the whole popup.
func (m *Manager) paint(hdc uintptr, w *window, now time.Time) {
	m.layout(hdc, w, now, true)
}

// layout draws the popup if draw is set and returns its height either
// way, recording the clickable areas.
func (m *Manager) layout(hdc uintptr, w *window, now time.Time, draw bool) int32 {
	if w.lobby != nil {
		return m.layoutLobby(hdc, w, now, draw)
	}
	dpi := w.dpi
	s := func(v int32) int32 { return scale(v, dpi) }
	pad := s(14)
	inner := rect{pad, pad, w.w - pad, w.h}
	var hits []hit
	put := func(str string, r rect, pt10 int32, bold bool, color uint32, flags uintptr) rect {
		if draw {
			m.text(hdc, str, r, pt10, bold, color, flags, dpi)
		}
		return m.text(hdc, str, r, pt10, bold, color, flags|dtCalcRect, dpi)
	}
	if draw {
		roundRect(hdc, rect{0, 0, w.w, w.h}, s(16), colBg, colBorder)
	}
	y := inner.top

	if q := w.quick; q != nil {
		remaining := time.Duration(q.Seconds)*time.Second - now.Sub(w.started)
		if remaining < 0 {
			remaining = 0
		}
		put("EINORDNEN", rect{inner.left, y, inner.right, y + s(16)}, 85, true, colMuted, dtSingleLine)
		put(strconv.Itoa(int((remaining+999*time.Millisecond)/time.Second))+" s", rect{inner.left, y, inner.right, y + s(16)}, 85, false, colMuted, dtSingleLine|dtRight)
		y += s(18)
		put(q.Title, rect{inner.left, y, inner.right, y + s(20)}, 100, false, colText, dtSingleLine|dtEndEllipsis)
		y += s(20)
		put("→ "+q.Target, rect{inner.left, y, inner.right, y + s(24)}, 125, true, colText, dtSingleLine|dtEndEllipsis)
		y += s(28)
		if draw {
			fill(hdc, rect{inner.left, y, inner.right, y + s(4)}, colTrack)
			total := float64(q.Seconds) * float64(time.Second)
			done := int32(float64(inner.right-inner.left) * float64(remaining) / max(total, 1))
			fill(hdc, rect{inner.left, y, inner.left + done, y + s(4)}, colAccent)
		}
		y += s(14)
		for i, label := range q.Options {
			if label == "" {
				continue
			}
			row := rect{inner.left - s(6), y, inner.right + s(6), y + s(26)}
			choice := i + 1
			idx := len(hits)
			hits = append(hits, hit{r: row, run: func() {
				if w.pick != nil {
					go w.pick(choice)
				}
			}})
			if draw && w.hot == idx {
				roundRect(hdc, row, s(8), colChip, colChip)
			}
			key := q.Keys[i]
			if key == "" {
				key = strconv.Itoa(choice)
			}
			chip := rect{inner.left, y + s(3), inner.left + s(96), y + s(23)}
			if draw {
				roundRect(hdc, chip, s(6), colChipHot, colChipHot)
			}
			put(key, rect{chip.left + s(4), chip.top, chip.right - s(4), chip.bottom}, 85, true, colText, dtSingleLine|dtCenter|dtVCenter|dtEndEllipsis)
			put(label, rect{chip.right + s(10), y, inner.right, y + s(26)}, 100, false, colText, dtSingleLine|dtVCenter|dtEndEllipsis)
			y += s(28)
		}
		y += s(4)
		put("Ohne Auswahl: "+q.Target, rect{inner.left, y, inner.right, y + s(18)}, 85, false, colMuted, dtSingleLine|dtEndEllipsis)
		y += s(18)
	}

	if t := w.toast; t != nil {
		closeR := rect{inner.right - s(20), y - s(4), inner.right + s(4), y + s(20)}
		hits = append(hits, hit{r: closeR, run: w.close})
		if draw && w.hot == len(hits)-1 {
			roundRect(hdc, closeR, s(6), colChip, colChip)
		}
		put("×", closeR, 120, false, colMuted, dtSingleLine|dtCenter|dtVCenter)
		tr := put(t.Title, rect{inner.left, y, inner.right - s(26), y + s(22)}, 105, true, colText, dtSingleLine|dtEndEllipsis)
		y = tr.bottom + s(4)
		br := put(t.Body, rect{inner.left, y, inner.right, y + s(400)}, 100, false, colText, dtWordBreak)
		y = br.bottom + s(12)
		x := inner.left
		for i, a := range t.Actions {
			if i == 2 {
				break
			}
			label := m.text(hdc, a.Label, rect{0, 0, s(300), s(28)}, 95, i == 0, colText, dtSingleLine|dtCalcRect, dpi)
			btn := rect{x, y, x + (label.right - label.left) + s(24), y + s(30)}
			run := a.Run
			hits = append(hits, hit{r: btn, run: func() {
				if run != nil {
					go run()
				}
				w.close()
			}})
			hot := draw && w.hot == len(hits)-1
			if draw {
				bg, fg := uint32(colChip), uint32(colText)
				if i == 0 {
					bg, fg = colAccent, colOnAccnt
				}
				if hot {
					if i == 0 {
						bg = 0x00ffa877
					} else {
						bg = colChipHot
					}
				}
				roundRect(hdc, btn, s(8), bg, bg)
				m.text(hdc, a.Label, btn, 95, i == 0, fg, dtSingleLine|dtCenter|dtVCenter, dpi)
			}
			x = btn.right + s(8)
		}
		if len(t.Actions) > 0 {
			y += s(30) + s(10)
		}
		if t.Disable != nil {
			lr := put("Nicht mehr anzeigen", rect{inner.left, y, inner.right, y + s(18)}, 85, false, colMuted, dtSingleLine)
			lr.bottom = lr.top + s(18)
			disable := t.Disable
			hits = append(hits, hit{r: lr, run: func() {
				go disable()
				w.close()
			}})
			if draw && w.hot == len(hits)-1 {
				put("Nicht mehr anzeigen", rect{inner.left, y, inner.right, y + s(18)}, 85, false, colText, dtSingleLine)
			}
			y += s(18)
		}
	}
	w.hits = hits
	return y + pad
}

// The scoreboard of the lobby, in pixels at 96 dpi.
const (
	lobbyW    = 800
	lobbyMid  = 120 // the middle column with the title and the averages
	lobbyCard = 90  // height of a player
	lobbyLogo = 34
)

// layoutLobby lays out the lobby like the scoreboard of the game: blue at
// the left, orange at the right, every player with name, rank logo, MMR in
// the playlist and wins, the averages in the middle and a countdown at the
// bottom. A strip under it shows the smurf, the state of the lookup and the
// hotkeys; the window is cut to the two.
func (m *Manager) layoutLobby(hdc uintptr, w *window, now time.Time, draw bool) int32 {
	l, dpi := w.lobby, w.dpi
	s := func(v int32) int32 { return scale(v, dpi) }
	font := func(px, weight int32) uintptr { return m.fontPx(s(px), weight) }
	put := func(str string, r rect, f uintptr, color uint32, flags uintptr) {
		if draw {
			drawText(hdc, str, r, f, color, flags|dtSingleLine)
		}
	}
	width := func(str string, f uintptr) int32 {
		r := drawText(hdc, str, rect{0, 0, w.w, s(40)}, f, 0, dtSingleLine|dtCalcRect)
		return r.right - r.left
	}
	small, smallBold := font(11, 400), font(11, 600)

	mid := s(lobbyMid)
	half := (w.w - mid) / 2
	rowH := s(10 + lobbyCard + 10)
	board := rect{0, 0, w.w, rowH + s(3) + 1} // the edge takes the last pixel
	if draw {
		fill(hdc, board, lobBg)
		fill(hdc, rect{half, 0, half + mid, rowH}, lobSurface)
		fill(hdc, rect{0, 1, half, 1 + s(2)}, lobBlue)
		fill(hdc, rect{half + mid, 1, w.w, 1 + s(2)}, lobOrange)
		fill(hdc, rect{0, rowH, w.w, rowH + s(3)}, lobTrack)
		from, to := lobbyBar(l.Busy, l.Seconds, now.Sub(w.shown))
		fill(hdc, rect{int32(from * float64(w.w)), rowH, int32(to * float64(w.w)), rowH + s(3)}, lobAccent)
	}

	// the middle: the title, under it the averages on one baseline and
	// which playlist they are
	cur := min(max(l.Rank, 1), 3) - 1
	big := font(15, 400)
	bigUp, bigLine := ascent(hdc, big)
	smallUp, _ := ascent(hdc, small)
	top := (rowH - s(17+3+23)) / 2
	if draw {
		procSetTextCharExtra.Call(hdc, uintptr(s(1)))
		put(strings.ToUpper(l.Title), rect{half, top, half + mid, top + s(17)}, small, lobMuted, dtCenter|dtVCenter)
		procSetTextCharExtra.Call(hdc, 0)
	}
	avg := l.Avg
	for i := range avg {
		if avg[i] == "" {
			avg[i] = "–"
		}
	}
	of := "Ø " + strconv.Itoa(cur+1) + "s"
	wb, wa, wo := width(avg[0], big), width(of, small), width(avg[1], big)
	base := top + s(17+3) + (s(23)-bigLine)/2 + bigUp
	x := half + (mid-wb-wa-wo-s(16))/2
	put(avg[0], rect{x, base - bigUp, x + wb, base + bigLine}, big, lobBlue, 0)
	x += wb + s(8)
	put(of, rect{x, base - smallUp, x + wa, base + bigLine}, small, lobMuted, 0)
	x += wa + s(8)
	put(avg[1], rect{x, base - bigUp, x + wo, base + bigLine}, big, lobOrange, 0)

	// the players, side by side in their half
	name, nameMe, mmr := font(12, 400), font(12, 600), font(17, 600)
	logo := s(lobbyLogo)
	for team, rows := range l.Teams {
		slots := int32(max(l.Size, len(rows), 1))
		left, right, align := s(12), half-s(10), uintptr(0)
		if team == 1 {
			left, right, align = half+mid+s(10), w.w-s(12), dtRight
		}
		cw := (right - left - s(4)*(slots-1)) / slots
		for i, p := range rows {
			x := left + int32(i)*(cw+s(4))
			card := rect{x, s(10), x + cw, s(10 + lobbyCard)}
			in := rect{card.left + s(8), card.top + s(6), card.right - s(8), card.bottom - s(6)}
			f, color := name, uint32(lobText)
			if p.Me {
				f, color = nameMe, lobMe
				if draw {
					roundRect(hdc, card, s(8), lobMeBg, lobMeBg)
				}
			}
			put(p.Name, rect{in.left, in.top, in.right, in.top + s(19)}, f, color, dtVCenter|dtEndEllipsis|align)

			// logo and MMR, mirrored for orange; "…" until the lookup answers
			r := p.Ranks[cur]
			text := r.MMR
			row := rect{in.left, in.top + s(23), in.right, in.top + s(23) + logo}
			if r.Label == "" {
				text = "…"
			} else {
				box := rect{row.left, row.top, row.left + logo, row.bottom}
				if team == 1 {
					box = rect{row.right - logo, row.top, row.right, row.bottom}
				}
				if draw && (r.MMR == "" || !m.drawLogo(hdc, r.Tier, box)) {
					outline(hdc, box, s(8), psDash, lobEdge)
					put(r.Label, box, smallBold, lobMuted, dtCenter|dtVCenter)
				}
				if team == 0 {
					row.left = box.right + s(8)
				} else {
					row.right = box.left - s(8)
				}
			}
			put(text, row, mmr, lobText, dtVCenter|align)
			if p.Wins != "" && r.Label != "" {
				put(p.Wins+" Siege", rect{in.left, in.top + s(61), in.right, in.top + s(78)}, small, lobMuted, dtVCenter|dtEndEllipsis|align)
			}
		}
	}

	// the strip: smurf, state of the lookup, hotkeys
	const (
		footText = iota
		footTag
		footKey
	)
	type piece struct {
		text   string
		font   uintptr
		color  uint32
		kind   int
		before int32 // space before it
		w      int32
	}
	var foot []piece
	gap, note := int32(0), -1
	add := func(text string, f uintptr, color uint32, kind int) {
		if len(foot) > 0 {
			gap += s(6)
		}
		pad := [3]int32{0, 2 * s(7), 2*s(5) + 2}[kind]
		foot = append(foot, piece{text, f, color, kind, gap, width(text, f) + pad})
		gap = 0
	}
	if l.Smurf {
		add("Smurf markiert", small, lobTag, footTag)
		gap = s(6)
	}
	if l.Note != "" {
		note = len(foot)
		add(l.Note, small, lobNote, footText)
		gap = s(6)
	}
	for i, h := range l.Hints {
		if i > 0 {
			gap = s(6)
		}
		add(h.Key, smallBold, lobKey, footKey)
		add(h.Action, small, lobNote, footText)
	}
	w.parts = []rect{board}
	if draw {
		outline(hdc, board, s(16), psSolid, lobEdge)
	}
	if len(foot) == 0 {
		return board.bottom
	}
	padX, padY, line := s(8), s(3), s(19)
	total := 2 * padX
	for _, p := range foot {
		total += p.before + p.w
	}
	if over := total - w.w; over > 0 && note >= 0 {
		foot[note].w = max(foot[note].w-over, 0) // a long error gets an ellipsis
		total = w.w
	}
	pill := rect{(w.w - total) / 2, board.bottom + s(6), (w.w + total) / 2, board.bottom + s(6) + line + 2*padY}
	w.parts = append(w.parts, pill)
	if draw {
		roundRect(hdc, pill, s(16), lobBg, lobEdge)
		x := pill.left + padX
		for _, p := range foot {
			x += p.before
			r := rect{x, pill.top + padY, x + p.w, pill.top + padY + line}
			switch p.kind {
			case footTag:
				roundRect(hdc, r, s(12), lobTagBg, lobTagBg)
				put(p.text, r, p.font, p.color, dtCenter|dtVCenter)
			case footKey:
				chip := rect{r.left, r.top + (line-s(17))/2, r.right, r.top + (line-s(17))/2 + s(17)}
				roundRect(hdc, chip, s(6), lobBg, lobEdge)
				put(p.text, chip, p.font, p.color, dtCenter|dtVCenter)
			default:
				put(p.text, r, p.font, p.color, dtVCenter|dtEndEllipsis)
			}
			x = r.right
		}
	}
	return pill.bottom
}
