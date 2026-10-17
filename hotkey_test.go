package main

import "testing"

func TestParseHotkey(t *testing.T) {
	tests := []struct {
		in      string
		want    hotkey
		wantErr bool
	}{
		{in: "Ctrl+Alt+B", want: hotkey{mods: modCtrl | modAlt, vk: 'B'}},
		{in: "strg + umschalt + f9", want: hotkey{mods: modCtrl | modShift, vk: 0x78}},
		{in: "Win+0", want: hotkey{mods: modWin, vk: '0'}},
		{in: "F24", want: hotkey{vk: 0x87}},
		{in: ""},
		{in: "Ctrl+B+C", wantErr: true},
		{in: "Alt+", wantErr: true},
		{in: "Ctrl+F25", wantErr: true},
		{in: "Hyper+B", wantErr: true},
	}
	for _, tc := range tests {
		got, err := parseHotkey(tc.in)
		if (err != nil) != tc.wantErr || (!tc.wantErr && got != tc.want) {
			t.Errorf("parseHotkey(%q) = %+v, %v, want %+v, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}
