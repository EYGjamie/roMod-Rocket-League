package desktop

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestPortalFilterSignature(t *testing.T) {
	if sig := dbus.SignatureOf([]fileFilter{globFilter("Replays", "*.replay")}).String(); sig != "a(sa(us))" {
		t.Errorf("filters have signature %s, the portal wants a(sa(us))", sig)
	}
}
