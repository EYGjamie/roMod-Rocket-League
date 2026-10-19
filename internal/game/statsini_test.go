package game

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// shipped is DefaultStatsAPI.ini as the game ships it: CRLF, no newline at
// the end and the API switched off.
const shipped = "[TAGame.MatchStatsExporter_TA]\r\n\r\n" +
	"; Port the client will listen for tcp connections on (must be different than WebPort, set to 0 to disable)\r\nPort=49123\r\n\r\n" +
	"; Port the client will listen for web connections on (must be different than Port, set to 0 to disable)\r\nWebPort=49124\r\n\r\n" +
	"; How many times per second the game sends the update state (capped at 120, 0 disables this feature)\r\nPacketSendRate=0"

func TestParseStatsINI(t *testing.T) {
	for _, c := range []struct {
		in         string
		rate, port int
	}{
		{shipped, 0, 49123},
		{"[TAGame.MatchStatsExporter_TA]\nPacketSendRate=30\nPort=50000\n", 30, 50000},
		{"[Other]\nPacketSendRate=5\n[tagame.matchstatsexporter_ta]\n PacketSendRate = 2 \n", 2, 49123},
		{"", 0, 49123},
	} {
		if rate, port := parseStatsINI([]byte(c.in)); rate != c.rate || port != c.port {
			t.Errorf("parseStatsINI(%q) = %d, %d, want %d, %d", c.in, rate, port, c.rate, c.port)
		}
	}
}

func TestSetStatsINI(t *testing.T) {
	got := string(setStatsINI([]byte(shipped), 1, 49123))
	want := strings.Replace(shipped, "PacketSendRate=0", "PacketSendRate=1", 1) + "\r\n"
	if got != want {
		t.Errorf("shipped file:\n%q\nwant\n%q", got, want)
	}

	got = string(setStatsINI([]byte("[Core]\nA=1\n\n[TAGame.MatchStatsExporter_TA]\nWebPort=1\n\n[Next]\nB=2\n"), 1, 49123))
	want = "[Core]\nA=1\n\n[TAGame.MatchStatsExporter_TA]\nWebPort=1\nPacketSendRate=1\nPort=49123\n\n[Next]\nB=2\n"
	if got != want {
		t.Errorf("keys missing:\n%q\nwant\n%q", got, want)
	}

	got = string(setStatsINI(nil, 1, 49123))
	want = "[TAGame.MatchStatsExporter_TA]\r\nPacketSendRate=1\r\nPort=49123\r\n"
	if got != want {
		t.Errorf("new file: %q, want %q", got, want)
	}

	got = string(setStatsINI([]byte("[Core]\nA=1"), 1, 49123))
	want = "[Core]\nA=1\n\n[TAGame.MatchStatsExporter_TA]\nPacketSendRate=1\nPort=49123\n"
	if got != want {
		t.Errorf("section missing: %q, want %q", got, want)
	}
}

func TestEnableStatsAPI(t *testing.T) {
	game := t.TempDir()
	cfg := filepath.Join(game, "TAGame", "Config")
	os.MkdirAll(cfg, 0o755)
	def := filepath.Join(cfg, "DefaultStatsAPI.ini")
	os.WriteFile(def, []byte(shipped), 0o644)

	list := StatsINIs([]string{game, filepath.Join(game, "fehlt"), game})
	if len(list) != 1 || list[0].Path != def || !list[0].Exists || list[0].On() {
		t.Fatalf("StatsINIs() = %+v", list)
	}
	if err := EnableStatsAPI(def, 50000); err != nil {
		t.Fatal(err)
	}
	s, _ := ReadStatsINI(def)
	if !s.On() || s.Rate != 1 || s.Port != 49123 {
		t.Errorf("after EnableStatsAPI: %+v, the port in the file stays", s)
	}

	// TAStatsAPI.ini wins once it exists; a rate that is set stays
	ta := filepath.Join(cfg, "TAStatsAPI.ini")
	os.WriteFile(ta, []byte("[TAGame.MatchStatsExporter_TA]\nPacketSendRate=60\nPort=0\n"), 0o644)
	if list := StatsINIs([]string{game}); len(list) != 1 || list[0].Path != ta || list[0].On() {
		t.Fatalf("StatsINIs() = %+v", list)
	}
	if err := EnableStatsAPI(ta, 50000); err != nil {
		t.Fatal(err)
	}
	if s, _ := ReadStatsINI(ta); s.Rate != 60 || s.Port != 50000 {
		t.Errorf("after EnableStatsAPI: %+v", s)
	}

	for _, bad := range []string{filepath.Join(game, "TAStatsAPI.ini"), filepath.Join(cfg, "Engine.ini")} {
		if err := EnableStatsAPI(bad, 1); !errors.Is(err, ErrNotStatsINI) {
			t.Errorf("EnableStatsAPI(%s) = %v", bad, err)
		}
	}
}

func TestInstallFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(content), 0o644)
		return p
	}
	vdf := filepath.Join(dir, "steamapps")
	os.MkdirAll(vdf, 0o755)
	write("steamapps/libraryfolders.vdf", `"libraryfolders" { "0" { "path" "C:\\Program Files (x86)\\Steam" } "1" { "path" "F:\\SteamLibrary" } }`)
	if got := steamLibraryFolders(dir); !slices.Equal(got, []string{`C:\Program Files (x86)\Steam`, `F:\SteamLibrary`}) {
		t.Errorf("steamLibraryFolders() = %q", got)
	}

	item := write("a.item", `{"AppName": "Sugar", "DisplayName": "Rocket League®", "InstallLocation": "F:\\EpicGames\\rocketleague"}`)
	other := write("b.item", `{"AppName": "Fortnite", "InstallLocation": "C:\\Fortnite"}`)
	dat := write("LauncherInstalled.dat", `{"InstallationList": [{"InstallLocation": "D:\\Epic\\rocketleague", "AppName": "Sugar"}]}`)
	if got := epicInstalls([]string{item, other}, dat); !slices.Equal(got, []string{`F:\EpicGames\rocketleague`, `D:\Epic\rocketleague`}) {
		t.Errorf("epicInstalls() = %q", got)
	}

	heroic := write("installed.json", `{"Sugar": {"app_name": "Sugar", "install_path": "/home/x/Games/Heroic/rocketleague"}, "Other": {"install_path": "/x"}}`)
	if got := heroicInstalls(heroic, filepath.Join(dir, "fehlt.json")); !slices.Equal(got, []string{"/home/x/Games/Heroic/rocketleague"}) {
		t.Errorf("heroicInstalls() = %q", got)
	}
}
