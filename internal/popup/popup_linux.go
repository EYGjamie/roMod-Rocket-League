//go:build linux

package popup

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/EYGjamie/roMod-Rocket-League/internal/assets"
)

const (
	busName   = "org.freedesktop.Notifications"
	busPath   = "/org/freedesktop/Notifications"
	toastLife = 8 * time.Second
)

// Manager shows notifications through the desktop's notification service,
// which GNOME, KDE, Xfce and most others provide. Where the service shows
// buttons, toasts get their actions and the quick-sort prompt offers the
// alternatives as buttons.
type Manager struct {
	mu      sync.Mutex
	conn    *dbus.Conn
	tried   time.Time // last attempt to connect
	actions bool      // the service shows buttons
	markup  bool      // the service reads markup in the body
	runs    map[uint32]map[string]func()
	quick   uint32 // ID of the quick-sort prompt
	lobby   uint32 // ID of the lobby notification
}

func New() *Manager { return &Manager{runs: map[uint32]map[string]func(){}} }

// service connects on first use, and again a minute after a failure (the
// service may start later than the tool).
func (m *Manager) service() *dbus.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn != nil || time.Since(m.tried) < time.Minute {
		return m.conn
	}
	m.tried = time.Now()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("Benachrichtigungen nicht verfügbar: %v", err)
		return nil
	}
	var caps []string
	if err := conn.Object(busName, busPath).Call(busName+".GetCapabilities", 0).Store(&caps); err != nil {
		log.Printf("Benachrichtigungen nicht verfügbar: %v", err)
		conn.Close()
		return nil
	}
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(busPath), dbus.WithMatchInterface(busName)); err != nil {
		log.Printf("Knöpfe in Benachrichtigungen nicht verfügbar: %v", err)
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	go m.listen(signals)
	m.conn, m.actions, m.markup = conn, slices.Contains(caps, "actions"), slices.Contains(caps, "body-markup")
	return conn
}

// listen runs the actions the user clicks and forgets closed notifications.
func (m *Manager) listen(signals chan *dbus.Signal) {
	for sig := range signals {
		if len(sig.Body) < 2 {
			continue
		}
		id, _ := sig.Body[0].(uint32)
		m.mu.Lock()
		switch sig.Name {
		case busName + ".ActionInvoked":
			key, _ := sig.Body[1].(string)
			if run := m.runs[id][key]; run != nil {
				go run()
			}
			delete(m.runs, id) // one choice per notification
		case busName + ".NotificationClosed":
			delete(m.runs, id)
			if m.quick == id {
				m.quick = 0
			}
		}
		m.mu.Unlock()
	}
}

// Interactive reports whether notifications can show buttons, which the
// quick-sort prompt needs.
func (m *Manager) Interactive() bool {
	if m.service() == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.actions
}

type button struct {
	key, label string
	run        func()
}

// notify shows a notification and returns its ID, 0 if none was shown.
func (m *Manager) notify(summary, body string, buttons []button, hints map[string]dbus.Variant, timeout time.Duration) uint32 {
	conn := m.service()
	if conn == nil {
		return 0
	}
	m.mu.Lock()
	withButtons, markup := m.actions, m.markup
	m.mu.Unlock()
	if markup {
		body = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(body)
	}
	actions := []string{}
	runs := map[string]func(){}
	if withButtons {
		for _, b := range buttons {
			actions = append(actions, b.key, b.label)
			runs[b.key] = b.run
		}
	}
	if hints == nil {
		hints = map[string]dbus.Variant{}
	}
	if img, ok := iconData(); ok {
		hints["image-data"] = dbus.MakeVariant(img)
	}
	var id uint32
	err := conn.Object(busName, busPath).Call(busName+".Notify", 0, "roMod", uint32(0), "", summary, body,
		actions, hints, int32(timeout/time.Millisecond)).Store(&id)
	if err != nil {
		log.Printf("Benachrichtigung nicht gezeigt: %v", err)
		return 0
	}
	m.mu.Lock()
	m.runs[id] = runs
	m.mu.Unlock()
	return id
}

func (m *Manager) close(id uint32) {
	if conn := m.service(); conn != nil && id != 0 {
		conn.Object(busName, busPath).Go(busName+".CloseNotification", 0, nil, id)
	}
}

// ShowQuickSort shows the prompt with the alternatives as buttons; pick
// receives 1–4 for a click. Notifications show at most three buttons, so
// with three alternatives the third one is left to its hotkey.
func (m *Manager) ShowQuickSort(q Quick, pick func(int)) {
	var buttons []button
	for i, label := range q.Options {
		if label != "" {
			choice := i + 1
			buttons = append(buttons, button{strconv.Itoa(choice), label, func() { pick(choice) }})
		}
	}
	if len(buttons) > 3 {
		buttons = append(buttons[:2], buttons[len(buttons)-1])
	}
	body := q.Title + "\nOhne Auswahl in " + strconv.Itoa(q.Seconds) + " s: " + q.Target
	id := m.notify("Einordnen → "+q.Target, body, buttons,
		map[string]dbus.Variant{"transient": dbus.MakeVariant(true)}, time.Duration(q.Seconds)*time.Second)
	m.mu.Lock()
	old := m.quick
	m.quick = id
	m.mu.Unlock()
	m.close(old)
}

// HideQuickSort removes the prompt.
func (m *Manager) HideQuickSort() {
	m.mu.Lock()
	id := m.quick
	m.quick = 0
	m.mu.Unlock()
	m.close(id)
}

// ShowLobby shows the lobby as a notification and replaces the one shown
// before. The tool closes it, so it has no timeout of its own.
func (m *Manager) ShowLobby(l Lobby) {
	id := m.notify(l.Title, lobbyText(l), nil, map[string]dbus.Variant{"transient": dbus.MakeVariant(true)}, 0)
	m.mu.Lock()
	old := m.lobby
	m.lobby = id
	m.mu.Unlock()
	m.close(old)
}

// HideLobby removes the lobby notification.
func (m *Manager) HideLobby() {
	m.mu.Lock()
	id := m.lobby
	m.lobby = 0
	m.mu.Unlock()
	m.close(id)
}

// Toast shows a notification with its actions as buttons.
func (m *Manager) Toast(t Toast) {
	var buttons []button
	for i, a := range t.Actions {
		if i == 2 {
			break
		}
		buttons = append(buttons, button{strconv.Itoa(i), a.Label, a.Run})
	}
	if t.Disable != nil {
		buttons = append(buttons, button{"disable", "Nicht mehr anzeigen", t.Disable})
	}
	m.notify(t.Title, t.Body, buttons, nil, toastLife)
}

// pixmap is the (iiibiiay) image-data hint: width, height, row stride,
// alpha, bits per sample, channels and RGBA bytes.
type pixmap struct {
	Width, Height, Stride int32
	Alpha                 bool
	Bits, Channels        int32
	Data                  []byte
}

var iconData = sync.OnceValues(func() (pixmap, bool) {
	src, err := png.Decode(bytes.NewReader(assets.Icon))
	if err != nil {
		return pixmap{}, false
	}
	rgba := image.NewNRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	b := rgba.Bounds()
	return pixmap{int32(b.Dx()), int32(b.Dy()), int32(rgba.Stride), true, 8, 4, rgba.Pix}, true
})
