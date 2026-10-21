//go:build linux

package desktop

import (
	"context"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// TrayHint says where to find the icon.
const TrayHint = "Das Symbol findest du im Infobereich der Leiste (unter GNOME mit der Erweiterung „AppIndicator and KStatusNotifierItem Support“)."

// A left click on the icon opens the interface.
const leftClickOpens = true

// trayHost checks for the session bus the icon is announced on. Without a
// panel showing such icons (GNOME without the AppIndicator extension) the
// icon appears once one starts.
func trayHost() error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	if err == nil && !has {
		log.Print("Kein Infobereich für das Symbol gefunden; unter GNOME zeigt es die Erweiterung „AppIndicator and KStatusNotifierItem Support“")
	}
	return nil
}

func ShowError(msg string) { dialog(true, msg) }

func ShowInfo(msg string) { dialog(false, msg) }

// dialog shows msg with zenity or kdialog, whichever is installed. The
// message is in the log either way.
func dialog(isError bool, msg string) {
	var cmd *exec.Cmd
	switch {
	case installed("zenity"):
		kind := "--info"
		if isError {
			kind = "--error"
		}
		cmd = exec.Command("zenity", kind, "--no-markup", "--title=roMod", "--text="+msg)
	case installed("kdialog"):
		kind := "--msgbox"
		if isError {
			kind = "--error"
		}
		cmd = exec.Command("kdialog", "--title", "roMod", kind, msg)
	case installed("xmessage"):
		cmd = exec.Command("xmessage", "-center", msg)
	default:
		return
	}
	cmd.Run()
}

func installed(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Edit opens a text file with the user's default application.
func Edit(path string) error { return start("xdg-open", path) }

// OpenFolder opens a folder in the file manager.
func OpenFolder(dir string) error { return start("xdg-open", dir) }

// Reveal opens the file manager with the file selected, through the
// interface Nautilus, Dolphin, Nemo and others offer, or else its folder.
func Reveal(path string) error {
	if conn, err := dbus.SessionBus(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		uri := (&url.URL{Scheme: "file", Path: path}).String()
		call := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1").
			CallWithContext(ctx, "org.freedesktop.FileManager1.ShowItems", 0, []string{uri}, "")
		if call.Err == nil {
			return nil
		}
	}
	return OpenFolder(filepath.Dir(path))
}

// PickFiles lets the user choose replay files, starting in dir: with the
// desktop's own dialog through the file chooser portal, or else zenity or
// kdialog.
func PickFiles(dir string) []string {
	if paths, ok := pickPortal(dir); ok {
		return paths
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	var cmd *exec.Cmd
	switch {
	case installed("zenity"):
		cmd = exec.Command("zenity", "--file-selection", "--multiple", "--separator=\n", "--title=Replays hochladen",
			"--filename="+dir+"/", "--file-filter=Rocket-League-Replays | *.replay", "--file-filter=Alle Dateien | *")
	case installed("kdialog"):
		cmd = exec.Command("kdialog", "--title", "Replays hochladen", "--getopenfilename", dir,
			"*.replay|Rocket-League-Replays", "--multiple", "--separate-output")
	default:
		log.Print("Keine Dateiauswahl verfügbar; Replays lassen sich auch per Drag & Drop in der Oberfläche hochladen")
		return nil
	}
	out, err := cmd.Output()
	if err != nil {
		return nil // cancelled
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// fileFilter is the (sa(us)) filter of the file chooser portal.
type fileFilter struct {
	Name     string
	Patterns []struct {
		Kind    uint32 // 0 = glob pattern
		Pattern string
	}
}

func globFilter(name, pattern string) fileFilter {
	f := fileFilter{Name: name}
	f.Patterns = append(f.Patterns, struct {
		Kind    uint32
		Pattern string
	}{0, pattern})
	return f
}

// pickPortal asks xdg-desktop-portal for files. ok is false if there is no
// portal to ask.
func pickPortal(dir string) (paths []string, ok bool) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, false
	}
	defer conn.Close()
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return nil, false
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)

	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant("romod_" + strconv.FormatInt(time.Now().UnixNano(), 36)),
		"multiple":     dbus.MakeVariant(true),
		"filters":      dbus.MakeVariant([]fileFilter{globFilter("Rocket-League-Replays", "*.replay"), globFilter("Alle Dateien", "*")}),
	}
	if dir != "" {
		opts["current_folder"] = dbus.MakeVariant(append([]byte(dir), 0))
	}
	var handle dbus.ObjectPath
	err = conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop").
		Call("org.freedesktop.portal.FileChooser.OpenFile", 0, "", "Replays hochladen", opts).Store(&handle)
	if err != nil {
		return nil, false
	}
	for sig := range signals {
		if sig.Path != handle || sig.Name != "org.freedesktop.portal.Request.Response" || len(sig.Body) < 2 {
			continue
		}
		if code, _ := sig.Body[0].(uint32); code != 0 {
			return nil, true // cancelled
		}
		results, _ := sig.Body[1].(map[string]dbus.Variant)
		uris, _ := results["uris"].Value().([]string)
		for _, u := range uris {
			if p, err := url.Parse(u); err == nil && p.Scheme == "file" {
				paths = append(paths, p.Path)
			}
		}
		return paths, true
	}
	return nil, true
}
