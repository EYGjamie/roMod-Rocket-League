package popup

import (
	"bufio"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// sessionBus starts a private D-Bus session bus for the test.
func sessionBus(t *testing.T) {
	t.Helper()
	path, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon fehlt")
	}
	cmd := exec.Command(path, "--session", "--print-address", "--nofork", "--nopidfile")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Skip("dbus-daemon startet nicht")
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(addr))
}

type notification struct {
	id      uint32
	summary string
	body    string
	actions []string
	hints   map[string]dbus.Variant
	timeout int32
}

// server stands in for the desktop's notification service.
type server struct {
	conn   *dbus.Conn
	caps   []string
	mu     sync.Mutex
	shown  []notification
	closed []uint32
}

func (s *server) GetCapabilities() ([]string, *dbus.Error) { return s.caps, nil }

func (s *server) Notify(app string, replaces uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uint32(len(s.shown) + 1)
	s.shown = append(s.shown, notification{id, summary, body, actions, hints, timeout})
	return id, nil
}

func (s *server) CloseNotification(id uint32) *dbus.Error {
	s.mu.Lock()
	s.closed = append(s.closed, id)
	s.mu.Unlock()
	return nil
}

func (s *server) last(t *testing.T) notification {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.shown) == 0 {
		t.Fatal("no notification shown")
	}
	return s.shown[len(s.shown)-1]
}

func (s *server) click(id uint32, key string) {
	s.conn.Emit(busPath, busName+".ActionInvoked", id, key)
}

func startServer(t *testing.T, caps ...string) *server {
	t.Helper()
	sessionBus(t)
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s := &server{conn: conn, caps: caps}
	if err := conn.Export(s, busPath, busName); err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName = %v, %v", reply, err)
	}
	return s
}

func waitFor(t *testing.T, ch chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Errorf("ran %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("%q did not run", want)
	}
}

func TestToastWithButtons(t *testing.T) {
	s := startServer(t, "actions", "body", "body-markup")
	m := New()
	if !m.Interactive() {
		t.Fatal("Interactive() = false with a service that shows buttons")
	}
	ran := make(chan string, 4)
	m.Toast(Toast{Title: "Replay hochgeladen", Body: "2s Ranked · DFH Stadium · Sieg 3:1 → <Ranked>",
		Actions: []Action{{"Auf ballchasing öffnen", func() { ran <- "open" }}, {"Anderer Ordner", func() { ran <- "move" }}},
		Disable: func() { ran <- "disable" }})
	n := s.last(t)
	if n.summary != "Replay hochgeladen" || n.body != "2s Ranked · DFH Stadium · Sieg 3:1 → &lt;Ranked&gt;" || n.timeout != 8000 {
		t.Errorf("shown %+v", n)
	}
	if want := []string{"0", "Auf ballchasing öffnen", "1", "Anderer Ordner", "disable", "Nicht mehr anzeigen"}; !slices.Equal(n.actions, want) {
		t.Errorf("actions %q, want %q", n.actions, want)
	}
	if img, ok := n.hints["image-data"].Value().([]any); !ok || len(img) != 7 || img[0] != int32(64) {
		t.Errorf("image-data hint = %v", n.hints["image-data"])
	}
	s.click(n.id, "1")
	waitFor(t, ran, "move")
	s.click(n.id, "0") // one choice per notification
	select {
	case got := <-ran:
		t.Errorf("second click ran %q", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestQuickSortButtons(t *testing.T) {
	s := startServer(t, "actions", "body")
	m := New()
	picked := make(chan string, 4)
	pick := func(choice int) { picked <- string(rune('0' + choice)) }
	m.ShowQuickSort(Quick{Title: "3s Ranked · Mannfield · Sieg 2:0", Target: "Ranked › Threes",
		Options: [4]string{"Privat", "Training", "Casual", "Unsortiert"}, Seconds: 10}, pick)
	n := s.last(t)
	if n.summary != "Einordnen → Ranked › Threes" || !strings.Contains(n.body, "Ohne Auswahl in 10 s: Ranked › Threes") || n.timeout != 10000 {
		t.Errorf("shown %+v", n)
	}
	// at most three buttons: the inbox stays, the third alternative goes
	if want := []string{"1", "Privat", "2", "Training", "4", "Unsortiert"}; !slices.Equal(n.actions, want) {
		t.Errorf("actions %q, want %q", n.actions, want)
	}
	if v, _ := n.hints["transient"].Value().(bool); !v {
		t.Error("prompt is not transient")
	}
	s.click(n.id, "4")
	waitFor(t, picked, "4")

	m.ShowQuickSort(Quick{Title: "x", Target: "y", Options: [4]string{"", "", "", "Unsortiert"}, Seconds: 5}, pick)
	second := s.last(t)
	m.HideQuickSort()
	time.Sleep(100 * time.Millisecond)
	s.mu.Lock()
	closed := slices.Clone(s.closed)
	s.mu.Unlock()
	if !slices.Contains(closed, second.id) {
		t.Errorf("closed %v, want %d", closed, second.id)
	}
}

func TestWithoutButtons(t *testing.T) {
	s := startServer(t, "body")
	m := New()
	if m.Interactive() {
		t.Error("Interactive() = true with a service without buttons")
	}
	m.Toast(Toast{Title: "Upload fehlgeschlagen", Body: "a & b", Actions: []Action{{"Details", func() {}}}, Disable: func() {}})
	if n := s.last(t); len(n.actions) != 0 || n.body != "a & b" {
		t.Errorf("shown %+v, want no buttons and the body as is", n)
	}
}

func TestIconHintSignature(t *testing.T) {
	img, ok := iconData()
	if !ok || img.Width != 64 || len(img.Data) != 64*64*4 {
		t.Fatalf("icon = %dx%d with %d bytes, %v", img.Width, img.Height, len(img.Data), ok)
	}
	if sig := dbus.SignatureOf(img).String(); sig != "(iiibiiay)" {
		t.Errorf("image-data has signature %s, want (iiibiiay)", sig)
	}
}
