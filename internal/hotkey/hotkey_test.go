package hotkey

import "testing"

func TestParseHotkey(t *testing.T) {
	tests := []struct {
		in      string
		want    Hotkey
		wantErr bool
	}{
		{in: "Ctrl+Alt+B", want: Hotkey{mods: modCtrl | modAlt, vk: 'B'}},
		{in: "strg + umschalt + f9", want: Hotkey{mods: modCtrl | modShift, vk: 0x78}},
		{in: "Win+0", want: Hotkey{mods: modWin, vk: '0'}},
		{in: "Super+Shift+Q", want: Hotkey{mods: modWin | modShift, vk: 'Q'}},
		{in: "F24", want: Hotkey{vk: 0x87}},
		{in: ""},
		{in: "Ctrl+B+C", wantErr: true},
		{in: "Alt+", wantErr: true},
		{in: "Ctrl+F25", wantErr: true},
		{in: "Hyper+B", wantErr: true},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in)
		if (err != nil) != tc.wantErr || (!tc.wantErr && got != tc.want) {
			t.Errorf("Parse(%q) = %+v, %v, want %+v, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}
