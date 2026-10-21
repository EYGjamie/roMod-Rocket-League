package desktop

import (
	"errors"
	"io/fs"
	"os"
)

// moveIfFree renames old to new unless new exists already.
func moveIfFree(old, new string) {
	if _, err := os.Lstat(new); errors.Is(err, fs.ErrNotExist) {
		os.Rename(old, new)
	}
}
