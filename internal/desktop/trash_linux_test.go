package desktop

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRecycleMovesIntoHomeTrash(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	dir := t.TempDir()
	for i := range 2 {
		path := filepath.Join(dir, "a b.replay")
		os.WriteFile(path, []byte{byte(i)}, 0o644)
		if err := Recycle(path); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("file still there: %v", err)
		}
	}
	trash := filepath.Join(data, "Trash")
	for i, name := range []string{"a b.replay", "a b.2.replay"} {
		got, err := os.ReadFile(filepath.Join(trash, "files", name))
		if err != nil || len(got) != 1 || got[0] != byte(i) {
			t.Errorf("%s in the trash: %v, %v", name, got, err)
		}
		info, _ := os.ReadFile(filepath.Join(trash, "info", name+".trashinfo"))
		want := "[Trash Info]\nPath=" + (&url.URL{Path: filepath.Join(dir, "a b.replay")}).EscapedPath() + "\nDeletionDate="
		if !strings.HasPrefix(string(info), want) || !strings.Contains(string(info), "%20") {
			t.Errorf("%s.trashinfo:\n%s\nwant prefix\n%s", name, info, want)
		}
	}
}

// A file on another drive goes to the trash at the top of that drive, as
// for Steam libraries on a second disk. /dev/shm stands in for the drive.
func TestRecycleUsesTrashOfOtherDrive(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	shm, err := os.Stat("/dev/shm")
	home, _ := os.Stat(data)
	topTrash := "/dev/shm/.Trash-" + strconv.Itoa(os.Getuid())
	if err != nil || deviceOf(shm) == deviceOf(home) {
		t.Skip("no second file system at /dev/shm")
	}
	if _, err := os.Lstat(topTrash); err == nil {
		t.Skip(topTrash + " exists already")
	}
	dir, err := os.MkdirTemp("/dev/shm", "aru-test-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(dir)
	defer os.RemoveAll(topTrash)

	path := filepath.Join(dir, "x.replay")
	os.WriteFile(path, []byte("x"), 0o644)
	if err := Recycle(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(topTrash, "files", "x.replay")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.ReadFile(filepath.Join(topTrash, "info", "x.replay.trashinfo"))
	if want := "Path=" + filepath.Base(dir) + "/x.replay\n"; !strings.Contains(string(info), want) {
		t.Errorf("trashinfo:\n%s\nwant %q (relative to the drive)", info, want)
	}
	if fi, _ := os.Stat(topTrash); fi.Mode().Perm() != 0o700 {
		t.Errorf("%s has mode %v, want 0700", topTrash, fi.Mode().Perm())
	}
}
