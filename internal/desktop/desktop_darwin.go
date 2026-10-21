//go:build darwin

package desktop

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// TrayHint says where to find the icon.
const TrayHint = "Das Symbol findest du in der Menüleiste."

// A left click opens the menu, as usual on macOS.
const leftClickOpens = false

func trayHost() error { return nil }

func ShowError(msg string) { alert("critical", msg) }

func ShowInfo(msg string) { alert("informational", msg) }

func alert(kind, msg string) {
	osascript(`on run argv
	activate
	display alert "roMod" message (item 1 of argv) as `+kind+` buttons {"OK"} default button "OK"
end run`, msg)
}

// osascript runs an AppleScript. Text goes in as arguments of its run
// handler, never into the script itself.
func osascript(script string, args ...string) (string, error) {
	out, err := exec.Command("osascript", append([]string{"-e", script}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// Edit opens a text file in the default text editor.
func Edit(path string) error { return start("open", "-t", path) }

// OpenFolder opens a folder in the Finder.
func OpenFolder(dir string) error { return start("open", dir) }

// Reveal opens the Finder with the file selected.
func Reveal(path string) error { return start("open", "-R", path) }

// PickFiles lets the user choose replay files, starting in dir.
func PickFiles(dir string) []string {
	var args []string
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		args = append(args, dir)
	}
	out, err := osascript(`on run argv
	activate
	if (count of argv) > 0 then
		set picked to choose file with prompt "Replays hochladen" default location (POSIX file (item 1 of argv)) with multiple selections allowed
	else
		set picked to choose file with prompt "Replays hochladen" with multiple selections allowed
	end if
	set out to ""
	repeat with f in picked
		set out to out & POSIX path of f & linefeed
	end repeat
	return out
end run`, args...)
	if err != nil {
		return nil // cancelled
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// Recycle moves a file to the Trash. It never deletes: if nothing works
// the file stays and the error says why.
func Recycle(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	// Newer versions of macOS have a trash command.
	if _, err := os.Stat("/usr/bin/trash"); err == nil && exec.Command("/usr/bin/trash", abs).Run() == nil {
		return nil
	}
	if moveToTrash(abs, fi) == nil {
		return nil
	}
	// The Finder asks once whether the tool may control it.
	if _, err := osascript(`on run argv
	tell application "Finder" to delete ((POSIX file (item 1 of argv)) as alias)
end run`, abs); err != nil {
		return errors.New("Papierkorb: " + err.Error())
	}
	return nil
}

// moveToTrash moves the file into the Trash folder of its volume, under a
// free name.
func moveToTrash(path string, fi fs.FileInfo) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dev := deviceOf(fi)
	trash := filepath.Join(home, ".Trash")
	if t, err := os.Stat(trash); err != nil || deviceOf(t) != dev {
		trash = filepath.Join(mountTop(dev, filepath.Dir(path)), ".Trashes", strconv.Itoa(os.Getuid()))
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	for n := 1; n < 1000; n++ {
		name := base
		if n > 1 {
			name = strings.TrimSuffix(base, ext) + " " + strconv.Itoa(n) + ext
		}
		err := unix.RenamexNp(path, filepath.Join(trash, name), unix.RENAME_EXCL)
		if !errors.Is(err, unix.EEXIST) {
			return err
		}
	}
	return errors.New("kein freier Name im Papierkorb")
}
