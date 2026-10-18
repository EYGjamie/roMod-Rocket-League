package desktop

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestExecLineQuotes(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"/opt/aru/auto-replay-upload"}, `/opt/aru/auto-replay-upload`},
		{[]string{"/home/a b/auto-replay-upload", "-config", "/home/a b/c.json"}, `"/home/a b/auto-replay-upload" -config "/home/a b/c.json"`},
		{[]string{"/x/100%", `/x/"q"$y`}, `/x/100%% "/x/\\"q\\"\\$y"`},
		{[]string{`/x\y`}, `"/x\\\\y"`},
	}
	for _, tc := range tests {
		if got := execLine(tc.args); got != tc.want {
			t.Errorf("execLine(%q) = %s, want %s", tc.args, got, tc.want)
		}
	}
}

func TestDesktopEntryOnAndOff(t *testing.T) {
	on := desktopEntry([]string{"/bin/aru"}, false)
	if !strings.Contains(on, "\nExec=/bin/aru\n") || entrySwitchedOff(on) {
		t.Errorf("entry switched on:\n%s", on)
	}
	if off := desktopEntry([]string{"/bin/aru"}, true); !entrySwitchedOff(off) {
		t.Errorf("entry switched off:\n%s", off)
	}
	if !entrySwitchedOff("[Desktop Entry]\nHidden = true\n") || !entrySwitchedOff("X-GNOME-Autostart-enabled=false") {
		t.Error("desktop settings switching off not recognized")
	}
}

func TestAgentPlistIsValid(t *testing.T) {
	data := agentPlist([]string{"/Applications/A & B.app/Contents/MacOS/auto-replay-upload", "-config", "/Users/x/<c>.json"})
	var strs []string
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	inString := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("invalid plist: %v\n%s", err, data)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			inString = tok.Name.Local == "string"
		case xml.CharData:
			if inString {
				strs = append(strs, string(tok))
			}
		case xml.EndElement:
			inString = false
		}
	}
	want := []string{launchAgent, "/Applications/A & B.app/Contents/MacOS/auto-replay-upload", "-config", "/Users/x/<c>.json", "Aqua", "Interactive"}
	if strings.Join(strs, "|") != strings.Join(want, "|") {
		t.Errorf("strings = %q, want %q", strs, want)
	}
}
