//go:build !windows

package config

import "encoding/base64"

// Without DPAPI the token is only encoded, not encrypted; the file is
// readable by the user alone.
func protect(s string) (string, error) { return base64.StdEncoding.EncodeToString([]byte(s)), nil }

func unprotect(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return string(b), err
}
