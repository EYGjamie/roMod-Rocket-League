package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwitchedOffEntryStaysOffButMovesWithProgram(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SetAutostart(true, []string{"/old/auto-replay-upload"}); err != nil {
		t.Fatal(err)
	}
	path, _ := autostartFile()
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), "X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false", 1)), 0o644)

	if err := SyncAutostart([]string{"/new/auto-replay-upload"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if Autostart() || !strings.Contains(string(data), "Exec=/new/auto-replay-upload") {
		t.Errorf("Autostart() = %v, entry:\n%s", Autostart(), data)
	}
	if err := SetAutostart(true, []string{"/new/auto-replay-upload"}); err != nil || !Autostart() {
		t.Errorf("switching on again: Autostart() = %v, err %v", Autostart(), err)
	}
	if filepath.Dir(path) != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autostart") {
		t.Errorf("entry at %s", path)
	}
}
