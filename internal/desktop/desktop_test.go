package desktop

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTooltipFits(t *testing.T) {
	if got := tooltip(""); got != "auto-replay-upload" {
		t.Errorf("tooltip(\"\") = %q", got)
	}
	long := tooltip("20:23:05 " + strings.Repeat("ä", 200))
	if n := len(utf16.Encode([]rune(long))); n > 127 || !strings.HasSuffix(long, "…") {
		t.Errorf("tooltip has %d UTF-16 units, want at most 127 ending in …", n)
	}
}

func TestStatusLogKeepsNewestLine(t *testing.T) {
	StatusLog.Write([]byte("20:00:00 eins\n"))
	StatusLog.Write([]byte("20:00:01 systray error: kaputt\n"))
	StatusLog.Write([]byte("20:00:02 zwei\n"))
	if got := <-statusCh; got != "20:00:02 zwei" {
		t.Errorf("status = %q, want the newest non-systray line", got)
	}
	select {
	case s := <-statusCh:
		t.Errorf("unexpected extra status %q", s)
	default:
	}
}
