//go:build darwin || linux

package desktop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// SignIn says when autostart runs the tool.
const SignIn = "bei der Anmeldung"

func SetDPIAware() {}

// lock holds the instance lock until the process exits.
var lock *os.File

// SingleInstance reports false if the tool already runs for this user with
// the same configuration file. The lock ends with the process, so a file
// left behind by a crash does not block the next start.
func SingleInstance(configPath string) bool {
	dir, name := os.Getenv("XDG_RUNTIME_DIR"), instanceKey(configPath)+".lock"
	if dir == "" {
		// per user on macOS, possibly shared elsewhere
		dir, name = os.TempDir(), fmt.Sprintf("%s-%d.lock", instanceKey(configPath), os.Getuid())
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true // cannot tell; better twice than not at all
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return !errors.Is(err, unix.EWOULDBLOCK)
	}
	lock = f
	return true
}

// menuTitle adds the shortcut in brackets; menus here have no column for it.
func menuTitle(title, hotkey string) string {
	if hotkey == "" {
		return title
	}
	return title + " (" + hotkey + ")"
}

// start runs a program in the background and reaps it when it exits.
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func deviceOf(fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}

// mountTop walks up from dir to the topmost folder still on device dev.
func mountTop(dev uint64, dir string) string {
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		fi, err := os.Stat(parent)
		if err != nil || deviceOf(fi) != dev {
			return dir
		}
		dir = parent
	}
}
