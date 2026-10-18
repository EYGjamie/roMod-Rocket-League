//go:build linux || darwin

package game

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIsRocketLeague(t *testing.T) {
	for name, want := range map[string]bool{
		"RocketLeague.exe": true,
		`Z:\home\jamie\.local\share\Steam\steamapps\common\rocketleague\Binaries\Win64\RocketLeague.exe`: true,
		"/opt/games/rocketleague.exe": true,
		"RocketLeague.ex":             true, // /proc/<pid>/comm, cut to 15 characters
		"rocketleague.exe":            true, // WM_CLASS from Wine
		"RocketLeagueReplay.exe":      false,
		"steam":                       false,
		"":                            false,
	} {
		if got := isRocketLeague(name); got != want {
			t.Errorf("isRocketLeague(%q) = %v", name, got)
		}
	}
}

func TestProcRunning(t *testing.T) {
	proc := t.TempDir()
	add := func(pid, comm, cmdline string) {
		dir := filepath.Join(proc, pid)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644)
	}
	add("1", "systemd", "/sbin/init\x00")
	add("812", "steam", "/home/j/.local/share/Steam/ubuntu12_32/steam\x00-silent\x00")
	os.MkdirAll(filepath.Join(proc, "self"), 0o755)
	if procRunning(proc) {
		t.Fatal("game found without it running")
	}
	add("4242", "wine64-preload", `C:\Program Files\Epic Games\rocketleague\Binaries\Win64\RocketLeague.exe`+"\x00-EpicPortal\x00")
	if !procRunning(proc) {
		t.Error("game not found by its command line")
	}
	os.RemoveAll(filepath.Join(proc, "4242"))
	add("4243", "RocketLeague.ex", "")
	if !procRunning(proc) {
		t.Error("game not found by its process name")
	}
}

func TestProcArgs(t *testing.T) {
	buf := binary.LittleEndian.AppendUint32(nil, 2)
	buf = append(buf, "/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/lib/wine/x86_64-unix/wine\x00\x00\x00\x00"...)
	buf = append(buf, "C:\\RL\\RocketLeague.exe\x00-nomovie\x00HOME=/Users/j\x00"...)
	exe, args := procArgs(buf)
	if exe != "/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/lib/wine/x86_64-unix/wine" || !slices.Equal(args, []string{`C:\RL\RocketLeague.exe`, "-nomovie"}) {
		t.Errorf("procArgs = %q, %q", exe, args)
	}
	if exe, args := procArgs([]byte{1}); exe != "" || args != nil {
		t.Errorf("procArgs(short) = %q, %q", exe, args)
	}
}
