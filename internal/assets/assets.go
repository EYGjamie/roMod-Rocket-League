// Package assets holds the tool's icon: a download arrow on a violet
// triangle, grey while paused and with a red dot after failed
// uploads. gen.go draws it; the program file (cmd/romod/winres)
// and the interface (internal/web/ui/icon.svg) use the same drawing.
package assets

//go:generate go run gen.go

import _ "embed"

// Icon is the icon as a 64×64 PNG, for notifications.
//
//go:embed icon.png
var Icon []byte
