//go:build linux

package desktop

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Recycle moves a file to the trash as the freedesktop.org specification
// describes, so the file manager can restore it: into the home trash, or
// for a file on another drive (a Steam library, say) into the trash at the
// top of that drive. It never deletes; if neither works the file stays.
func Recycle(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	dev := deviceOf(fi)
	trash, top, err := trashDir(dev, abs)
	if err != nil {
		return fmt.Errorf("Papierkorb: %w", err)
	}
	return moveToTrash(abs, trash, top, time.Now())
}

// homeTrash is $XDG_DATA_HOME/Trash.
func homeTrash() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "Trash"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "Trash"), nil
}

// trashDir finds the trash for a file on device dev. top is set for a trash
// at the top of a drive, where paths are stored relative to it.
func trashDir(dev uint64, path string) (trash, top string, err error) {
	home, err := homeTrash()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(home, 0o700); err == nil {
		if fi, err := os.Stat(home); err == nil && deviceOf(fi) == dev {
			return home, "", nil
		}
	}
	top = mountTop(dev, filepath.Dir(path))
	uid := strconv.Itoa(os.Getuid())
	// A shared .Trash made by the administrator must be a sticky folder,
	// not a link; then each user has a folder inside.
	if fi, err := os.Lstat(filepath.Join(top, ".Trash")); err == nil && fi.IsDir() && fi.Mode()&fs.ModeSticky != 0 {
		dir := filepath.Join(top, ".Trash", uid)
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return dir, top, nil
		}
	}
	dir := filepath.Join(top, ".Trash-"+uid)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return "", "", errors.New(dir + " ist kein Ordner")
	}
	return dir, top, nil
}

// moveToTrash reserves a name by creating its .trashinfo file first, then
// moves the file next to it.
func moveToTrash(path, trash, top string, now time.Time) error {
	files, info := filepath.Join(trash, "files"), filepath.Join(trash, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	stored := path
	if top != "" {
		if rel, err := filepath.Rel(top, path); err == nil {
			stored = rel
		}
	}
	entry := "[Trash Info]\nPath=" + (&url.URL{Path: stored}).EscapedPath() +
		"\nDeletionDate=" + now.Format("2006-01-02T15:04:05") + "\n"

	base := filepath.Base(path)
	ext := filepath.Ext(base)
	for n := 1; n < 1000; n++ {
		name := base
		if n > 1 {
			name = strings.TrimSuffix(base, ext) + "." + strconv.Itoa(n) + ext
		}
		infoPath := filepath.Join(info, name+".trashinfo")
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return err
		}
		_, err = f.WriteString(entry)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = renameNoReplace(path, filepath.Join(files, name))
		}
		if errors.Is(err, fs.ErrExist) {
			os.Remove(infoPath) // a leftover without info, try the next name
			continue
		}
		if err != nil {
			os.Remove(infoPath)
			return err
		}
		return nil
	}
	return errors.New("Papierkorb: kein freier Name für " + base)
}

// renameNoReplace renames without overwriting an existing file.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) { // file system without the flag
		if _, err := os.Lstat(to); err == nil {
			return fs.ErrExist
		}
		return os.Rename(from, to)
	}
	if errors.Is(err, unix.EEXIST) {
		return fs.ErrExist
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
