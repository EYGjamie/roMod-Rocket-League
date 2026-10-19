//go:build linux || darwin

package game

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinuxDemoDirs(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home [x]") // pattern characters in the path
	games := t.TempDir()                           // a Steam library on another drive
	steam := filepath.Join(home, ".local", "share", "Steam")
	prefix := func(lib string) string {
		return filepath.Join(lib, "steamapps", "compatdata", "252950", "pfx", "drive_c", "users", "steamuser", "Documents", "My Games", "Rocket League", "TAGame")
	}
	heroic := filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Rocket League", "drive_c", "users", "jamie", "Documents", "My Games", "Rocket League", "TAGame")
	mkdirs(t, filepath.Join(prefix(steam), "Demos"), filepath.Join(prefix(games), "Demos"), filepath.Join(heroic, "DemosEpic"),
		filepath.Join(home, ".steam"), filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Fortnite"))
	os.Symlink(steam, filepath.Join(home, ".steam", "steam")) // the usual link: counted once
	vdf := "\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\"" + steam + "\"\n\t}\n\t\"1\"\n\t{\n\t\t\"path\"\t\t\"" + games + "\"\n\t}\n}\n"
	os.WriteFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0o644)

	got := findDemoDirs(linuxPatterns(home))
	want := []string{
		filepath.Join(prefix(filepath.Join(home, ".steam", "steam")), "Demos"),
		filepath.Join(prefix(games), "Demos"),
		filepath.Join(heroic, "DemosEpic"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%q\nwant\n%q", got, want)
	}
}

func TestMacDemoDirs(t *testing.T) {
	home := t.TempDir()
	crossover := filepath.Join(home, "Library", "Application Support", "CrossOver", "Bottles", "Steam", "drive_c", "users", "crossover", "Documents", "My Games", "Rocket League", "TAGame")
	whisky := filepath.Join(home, "Library", "Containers", "com.isaacmarovitz.Whisky", "Bottles", "1A2B", "drive_c", "users", "crossover", "Documents", "My Games", "Rocket League", "TAGame")
	mkdirs(t, filepath.Join(crossover, "Demos"), filepath.Join(whisky, "DemosEpic"), filepath.Join(whisky, "Config"))
	got := findDemoDirs(macPatterns(home))
	want := []string{filepath.Join(crossover, "Demos"), filepath.Join(whisky, "DemosEpic")}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%q\nwant\n%q", got, want)
	}
	if got := findDemoDirs(macPatterns(t.TempDir())); len(got) != 0 {
		t.Errorf("empty home: found %q", got)
	}
}

func TestInstallDirs(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home [x]")
	steam := filepath.Join(home, ".local", "share", "Steam")
	bottle := filepath.Join(home, ".local", "share", "bottles", "bottles", "Epic", "drive_c")
	mkdirs(t, filepath.Join(steam, "steamapps"), filepath.Join(bottle, "Program Files", "Epic Games", "rocketleague"),
		filepath.Join(bottle, "users", "jamie", "Documents", "My Games", "Rocket League", "TAGame"))
	heroic := filepath.Join(home, "installed.json")
	os.WriteFile(heroic, []byte(`{"Sugar": {"app_name": "Sugar", "install_path": "/spiele/rocketleague"}}`), 0o644)

	got := installDirs(home, linuxPatterns, heroic)
	for _, want := range []string{
		filepath.Join(steam, "steamapps", "common", "rocketleague"),
		"/spiele/rocketleague",
		filepath.Join(home, "Games", "Heroic", "rocketleague"),
		filepath.Join(bottle, "Program Files", "Epic Games", "rocketleague"),
	} {
		if !slices.Contains(got, want) {
			t.Errorf("%s missing in\n%q", want, got)
		}
	}
}
