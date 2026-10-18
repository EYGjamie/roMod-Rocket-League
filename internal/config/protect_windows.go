//go:build windows

package config

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

// protect encrypts s for the current Windows user (DPAPI).
func protect(s string) (string, error) {
	in := []byte(s)
	var out windows.DataBlob
	err := windows.CryptProtectData(&windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return base64.StdEncoding.EncodeToString(unsafe.Slice(out.Data, out.Size)), nil
}

func unprotect(s string) (string, error) {
	in, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(in) == 0 {
		return "", err
	}
	var out windows.DataBlob
	err = windows.CryptUnprotectData(&windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return string(unsafe.Slice(out.Data, out.Size)), nil
}
