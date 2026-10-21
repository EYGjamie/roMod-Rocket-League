//go:build darwin || linux

package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSingleInstance(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "config.json")
	if !SingleInstance(cfg) {
		t.Fatal("first instance refused")
	}
	defer func() { lock.Close(); lock = nil }()
	if SingleInstance(cfg) {
		t.Error("second instance with the same configuration allowed")
	}
	if !SingleInstance(filepath.Join(t.TempDir(), "other.json")) {
		t.Error("instance with another configuration refused")
	}
}

func TestAutostartFollowsChoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	args := []string{"/opt/romod/romod"}

	if err := SyncAutostart(args); err != nil || !Autostart() {
		t.Fatalf("first start: Autostart() = %v, err %v", Autostart(), err)
	}
	if err := SetAutostart(false, args); err != nil || Autostart() {
		t.Fatalf("turned off: Autostart() = %v, err %v", Autostart(), err)
	}
	if err := SyncAutostart(args); err != nil || Autostart() {
		t.Fatalf("start after turning off: Autostart() = %v, err %v", Autostart(), err)
	}
	if err := SetAutostart(true, args); err != nil || !Autostart() {
		t.Fatalf("turned on: Autostart() = %v, err %v", Autostart(), err)
	}

	// removed in the system settings: the next start leaves it removed
	path, _ := autostartFile()
	os.Remove(path)
	if err := SyncAutostart(args); err != nil || Autostart() || entryExists() {
		t.Fatalf("start after removal: Autostart() = %v, err %v", Autostart(), err)
	}
}
