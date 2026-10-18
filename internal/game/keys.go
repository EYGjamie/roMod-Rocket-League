package game

import "fmt"

// scanCode is a key as the keyboard reports it; extended keys carry the
// E0 prefix.
type scanCode struct {
	code     uint16
	extended bool
}

// keyCodes maps the names browsers use for physical keys
// (KeyboardEvent.code) to scan codes, so the save key can be recorded in
// the settings page and sent the way the game reads it.
var keyCodes = map[string]scanCode{
	"Backspace": {0x0E, false}, "Tab": {0x0F, false}, "Enter": {0x1C, false}, "Space": {0x39, false},
	"CapsLock": {0x3A, false}, "ShiftLeft": {0x2A, false}, "ShiftRight": {0x36, false},
	"ControlLeft": {0x1D, false}, "ControlRight": {0x1D, true}, "AltLeft": {0x38, false}, "AltRight": {0x38, true},
	"Minus": {0x0C, false}, "Equal": {0x0D, false}, "BracketLeft": {0x1A, false}, "BracketRight": {0x1B, false},
	"Semicolon": {0x27, false}, "Quote": {0x28, false}, "Backquote": {0x29, false}, "Backslash": {0x2B, false},
	"Comma": {0x33, false}, "Period": {0x34, false}, "Slash": {0x35, false}, "IntlBackslash": {0x56, false},
	"Insert": {0x52, true}, "Delete": {0x53, true}, "Home": {0x47, true}, "End": {0x4F, true},
	"PageUp": {0x49, true}, "PageDown": {0x51, true},
	"ArrowUp": {0x48, true}, "ArrowDown": {0x50, true}, "ArrowLeft": {0x4B, true}, "ArrowRight": {0x4D, true},
	"Numpad0": {0x52, false}, "Numpad1": {0x4F, false}, "Numpad2": {0x50, false}, "Numpad3": {0x51, false},
	"Numpad4": {0x4B, false}, "Numpad5": {0x4C, false}, "Numpad6": {0x4D, false}, "Numpad7": {0x47, false},
	"Numpad8": {0x48, false}, "Numpad9": {0x49, false}, "NumpadDecimal": {0x53, false}, "NumpadAdd": {0x4E, false},
	"NumpadSubtract": {0x4A, false}, "NumpadMultiply": {0x37, false}, "NumpadDivide": {0x35, true}, "NumpadEnter": {0x1C, true},
	"F11": {0x57, false}, "F12": {0x58, false},
}

func init() {
	for i, c := range "QWERTYUIOP" {
		keyCodes["Key"+string(c)] = scanCode{uint16(0x10 + i), false}
	}
	for i, c := range "ASDFGHJKL" {
		keyCodes["Key"+string(c)] = scanCode{uint16(0x1E + i), false}
	}
	for i, c := range "ZXCVBNM" {
		keyCodes["Key"+string(c)] = scanCode{uint16(0x2C + i), false}
	}
	for d := 1; d <= 9; d++ {
		keyCodes[fmt.Sprintf("Digit%d", d)] = scanCode{uint16(0x01 + d), false}
	}
	keyCodes["Digit0"] = scanCode{0x0B, false}
	for f := 1; f <= 10; f++ {
		keyCodes[fmt.Sprintf("F%d", f)] = scanCode{uint16(0x3A + f), false}
	}
}

// ValidKey reports whether the save key can be sent.
func ValidKey(code string) bool {
	_, ok := keyCodes[code]
	return ok
}
