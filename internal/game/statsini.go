package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// The Stats API is off until its ini file in the game's install folder has
// a PacketSendRate above 0. The game reads TAGame/Config/TAStatsAPI.ini and
// falls back to DefaultStatsAPI.ini, only when it starts.

const statsSection = "TAGame.MatchStatsExporter_TA"

// DefaultStatsPort is the game's TCP port for the Stats API.
const DefaultStatsPort = 49123

// StatsINI is the Stats API setting of one installation.
type StatsINI struct {
	Path   string // the file the game reads
	Exists bool   // false: the file is created when switching on
	Rate   int    // PacketSendRate, 0 = off
	Port   int    // TCP port, 0 = off
}

// On reports whether the game opens the TCP port.
func (s StatsINI) On() bool { return s.Rate > 0 && s.Port > 0 }

// StatsINIs returns the Stats API setting of every installation among
// dirs (the game's install folders) that has a TAGame/Config folder.
func StatsINIs(dirs []string) []StatsINI {
	var out []StatsINI
	for _, dir := range unique(dirs) {
		cfg := filepath.Join(dir, "TAGame", "Config")
		if fi, err := os.Stat(cfg); err != nil || !fi.IsDir() {
			continue
		}
		path := filepath.Join(cfg, "TAStatsAPI.ini")
		if _, err := os.Stat(path); err != nil {
			if def := filepath.Join(cfg, "DefaultStatsAPI.ini"); fileExists(def) {
				path = def
			}
		}
		s, _ := ReadStatsINI(path)
		out = append(out, s)
	}
	return out
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// ReadStatsINI reads the Stats API setting from path. A missing file means
// the game's defaults: off, port 49123.
func ReadStatsINI(path string) (StatsINI, error) {
	s := StatsINI{Path: path, Port: DefaultStatsPort}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	} else if err != nil {
		return s, err
	}
	s.Exists = true
	s.Rate, s.Port = parseStatsINI(data)
	return s, nil
}

// parseStatsINI returns PacketSendRate and Port of the exporter section,
// with the game's defaults for missing keys.
func parseStatsINI(data []byte) (rate, port int) {
	port = DefaultStatsPort
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if !strings.EqualFold(section, statsSection) {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "PacketSendRate":
			rate = n
		case "Port":
			port = n
		}
	}
	return rate, port
}

// setStatsINI returns data with PacketSendRate and Port in the exporter
// section set, keeping everything else and the line endings.
func setStatsINI(data []byte, rate, port int) []byte {
	nl := "\r\n"
	if len(data) > 0 && !bytes.Contains(data, []byte("\r\n")) {
		nl = "\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	want := map[string]string{"PacketSendRate": strconv.Itoa(rate), "Port": strconv.Itoa(port)}
	order := []string{"PacketSendRate", "Port"}

	start, end := -1, len(lines) // the section's lines are start+1 … end-1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if start >= 0 {
				end = i
				break
			}
			if strings.EqualFold(t[1:len(t)-1], statsSection) {
				start = i
			}
		}
	}
	if start < 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "["+statsSection+"]")
		start, end = len(lines)-1, len(lines)
	}
	for i := start + 1; i < end; i++ {
		key, _, ok := strings.Cut(lines[i], "=")
		if v, set := want[strings.TrimSpace(key)]; ok && set {
			lines[i] = strings.TrimSpace(key) + "=" + v
			delete(want, strings.TrimSpace(key))
		}
	}
	var missing []string
	for _, k := range order {
		if v, ok := want[k]; ok {
			missing = append(missing, k+"="+v)
		}
	}
	// after the section's last setting, before blank lines that follow it
	at := end
	for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	lines = append(lines[:at], append(missing, lines[at:]...)...)
	return []byte(strings.Join(lines, nl) + nl)
}

// ErrNotStatsINI protects EnableStatsAPI, which may run with administrator
// rights, from writing anything but the Stats API settings.
var ErrNotStatsINI = errors.New("keine Stats-API-Datei des Spiels")

// EnableStatsAPI switches the Stats API on in the ini file at path: a
// PacketSendRate of 1 unless one is set, and port unless a port is set. A
// missing file is created. It fails with fs.ErrPermission when the folder
// needs administrator rights.
func EnableStatsAPI(path string, port int) error {
	name := strings.ToLower(filepath.Base(path))
	if name != "tastatsapi.ini" && name != "defaultstatsapi.ini" || !strings.EqualFold(filepath.Base(filepath.Dir(path)), "Config") {
		return fmt.Errorf("%s: %w", path, ErrNotStatsINI)
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	rate, cur := parseStatsINI(data)
	if rate <= 0 {
		rate = 1
	}
	if cur > 0 && len(data) > 0 {
		port = cur
	}
	return os.WriteFile(path, setStatsINI(data, rate, port), 0o644)
}

var vdfPath = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

// steamLibraryFolders returns the library folders listed in a Steam
// installation's libraryfolders.vdf, besides the installation itself.
func steamLibraryFolders(root string) []string {
	unescape := strings.NewReplacer(`\\`, `\`, `\"`, `"`)
	var libs []string
	for _, vdf := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
		data, _ := os.ReadFile(vdf)
		for _, m := range vdfPath.FindAllStringSubmatch(string(data), -1) {
			libs = append(libs, unescape.Replace(m[1]))
		}
	}
	return libs
}

// epicAppName is Rocket League in the Epic Games Store and Legendary.
const epicAppName = "Sugar"

// heroicInstalls reads the install folder of the game from Legendary's
// installed.json, which Heroic uses for Epic games.
func heroicInstalls(files ...string) []string {
	var out []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var games map[string]struct {
			AppName     string `json:"app_name"`
			InstallPath string `json:"install_path"`
		}
		if json.Unmarshal(data, &games) != nil {
			continue
		}
		for name, g := range games {
			if (name == epicAppName || g.AppName == epicAppName) && g.InstallPath != "" {
				out = append(out, g.InstallPath)
			}
		}
	}
	return out
}

// epicInstalls reads the game's install folder from the Epic Games
// Launcher's manifests (*.item) and LauncherInstalled.dat.
func epicInstalls(manifests []string, launcherInstalled string) []string {
	var out []string
	for _, f := range manifests {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m struct {
			AppName         string
			InstallLocation string
		}
		if json.Unmarshal(data, &m) == nil && m.AppName == epicAppName && m.InstallLocation != "" {
			out = append(out, m.InstallLocation)
		}
	}
	if data, err := os.ReadFile(launcherInstalled); err == nil {
		var l struct {
			InstallationList []struct {
				AppName         string
				InstallLocation string
			}
		}
		if json.Unmarshal(data, &l) == nil {
			for _, i := range l.InstallationList {
				if i.AppName == epicAppName && i.InstallLocation != "" {
					out = append(out, i.InstallLocation)
				}
			}
		}
	}
	return out
}

// unique drops paths that lead to the same place as an earlier one.
func unique(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		key := filepath.Clean(p)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			key = real
		}
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}
