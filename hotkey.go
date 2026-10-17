package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Modifier flags as RegisterHotKey expects them.
const (
	modAlt   = 0x1
	modCtrl  = 0x2
	modShift = 0x4
	modWin   = 0x8
)

type hotkey struct {
	mods uint32
	vk   uint32
}

// parseHotkey reads combinations like "Ctrl+Alt+B" or "Strg+Shift+F9".
// The key is a letter, a digit or F1–F24; an empty string means no hotkey.
func parseHotkey(s string) (hotkey, error) {
	var hk hotkey
	if strings.TrimSpace(s) == "" {
		return hk, nil
	}
	parts := strings.Split(s, "+")
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "ctrl", "strg", "control":
			hk.mods |= modCtrl
		case "alt":
			hk.mods |= modAlt
		case "shift", "umschalt":
			hk.mods |= modShift
		case "win":
			hk.mods |= modWin
		default:
			return hk, fmt.Errorf("unbekannte Taste %q in %q", p, s)
		}
	}
	key := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	switch {
	case len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9'):
		hk.vk = uint32(key[0]) // virtual-key codes equal ASCII here
	case len(key) > 1 && key[0] == 'F':
		n, err := strconv.Atoi(key[1:])
		if err != nil || n < 1 || n > 24 {
			return hk, fmt.Errorf("unbekannte Taste %q in %q", key, s)
		}
		hk.vk = 0x70 + uint32(n-1) // VK_F1
	default:
		return hk, fmt.Errorf("unbekannte Taste %q in %q", key, s)
	}
	return hk, nil
}
