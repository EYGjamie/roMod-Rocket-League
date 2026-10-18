//go:build linux || darwin

package game

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// isRocketLeague reports whether a program name or path, also a Windows
// path, is the game's .exe. Linux cuts process names to 15 characters.
func isRocketLeague(name string) bool {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.EqualFold(name, "RocketLeague.exe") || strings.EqualFold(name, "RocketLeague.ex")
}

// procRunning looks for the game in a /proc file system. Wine names the
// process after the .exe and puts the .exe's Windows path first on the
// command line.
func procRunning(proc string) bool {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err == nil && isGameProcess(proc, e.Name()) {
			return true
		}
	}
	return false
}

func isGameProcess(proc, pid string) bool {
	if comm, err := os.ReadFile(filepath.Join(proc, pid, "comm")); err == nil && isRocketLeague(strings.TrimSpace(string(comm))) {
		return true
	}
	cmdline, err := os.ReadFile(filepath.Join(proc, pid, "cmdline"))
	if err != nil {
		return false
	}
	argv0, _, _ := strings.Cut(string(cmdline), "\x00")
	return isRocketLeague(argv0)
}

// procArgs splits what macOS reports as kern.procargs2: the number of
// arguments, the executable's path, padding, then the arguments, each
// ending with a NUL.
func procArgs(buf []byte) (exe string, args []string) {
	if len(buf) < 4 {
		return "", nil
	}
	argc := int(binary.LittleEndian.Uint32(buf))
	exe, rest := cutNUL(buf[4:])
	rest = bytes.TrimLeft(rest, "\x00")
	for i := 0; i < argc && len(rest) > 0; i++ {
		var a string
		a, rest = cutNUL(rest)
		args = append(args, a)
	}
	return exe, args
}

func cutNUL(b []byte) (string, []byte) {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i]), b[i+1:]
	}
	return string(b), nil
}
