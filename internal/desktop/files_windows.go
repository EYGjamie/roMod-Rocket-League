//go:build windows

package desktop

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	comdlg32            = windows.NewLazySystemDLL("comdlg32.dll")
	procSHFileOperation = shell32.NewProc("SHFileOperationW")
	procGetOpenFileName = comdlg32.NewProc("GetOpenFileNameW")
)

// shFileOp mirrors SHFILEOPSTRUCTW on 64-bit Windows.
type shFileOp struct {
	hwnd     uintptr
	function uint32
	from     *uint16
	to       *uint16
	flags    uint16
	aborted  int32
	mappings uintptr
	title    *uint16
}

const (
	foDelete     = 0x3
	fofSilent    = 0x4
	fofNoConfirm = 0x10
	fofAllowUndo = 0x40
	fofNoErrorUI = 0x400
)

// sizeof(SHFILEOPSTRUCTW) is 56 and sizeof(OPENFILENAMEW) 152 on 64-bit
// Windows; these fail to compile otherwise.
var (
	_ [56 - unsafe.Sizeof(shFileOp{})]byte
	_ [unsafe.Sizeof(shFileOp{}) - 56]byte
	_ [152 - unsafe.Sizeof(openFileName{})]byte
	_ [unsafe.Sizeof(openFileName{}) - 152]byte
)

// Recycle moves a file to the recycle bin.
func Recycle(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list ends with two NULs
	op := shFileOp{function: foDelete, from: &from[0], flags: fofAllowUndo | fofNoConfirm | fofSilent | fofNoErrorUI}
	if r, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op))); r != 0 {
		return errors.New("Papierkorb: Fehler " + windows.Errno(r).Error())
	}
	if op.aborted != 0 {
		return errors.New("Papierkorb: abgebrochen")
	}
	return nil
}

// Reveal opens Explorer with the file selected.
func Reveal(path string) error {
	return exec.Command("explorer.exe", "/select,"+path).Start()
}

// OpenFolder opens a folder in Explorer.
func OpenFolder(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}

// openFileName mirrors OPENFILENAMEW on 64-bit Windows.
type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      uintptr
	reservedDW    uint32
	flagsEx       uint32
}

const (
	ofnAllowMultiSelect = 0x200
	ofnExplorer         = 0x80000
	ofnFileMustExist    = 0x1000
	ofnNoChangeDir      = 0x8
)

// PickFiles lets the user choose replay files, starting in dir.
func PickFiles(dir string) []string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	defer windows.CoUninitialize()

	buf := make([]uint16, 64<<10)
	filter := utf16z("Rocket-League-Replays (*.replay)\x00*.replay\x00Alle Dateien\x00*.*\x00\x00")
	ofn := openFileName{file: &buf[0], maxFile: uint32(len(buf)), filter: &filter[0], filterIndex: 1,
		title:      windows.StringToUTF16Ptr("Replays hochladen"),
		initialDir: windows.StringToUTF16Ptr(dir),
		flags:      ofnAllowMultiSelect | ofnExplorer | ofnFileMustExist | ofnNoChangeDir}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return nil // cancelled
	}
	// one file: "C:\path\x.replay\0\0"; several: "C:\path\0a.replay\0b.replay\0\0"
	var parts []string
	for start, i := 0, 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			parts = append(parts, windows.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	if len(parts) <= 1 {
		return parts
	}
	var out []string
	for _, name := range parts[1:] {
		out = append(out, filepath.Join(parts[0], name))
	}
	return out
}

// utf16z encodes s, which carries its own NULs (UTF16FromString rejects those).
func utf16z(s string) []uint16 {
	return utf16.Encode([]rune(s))
}
