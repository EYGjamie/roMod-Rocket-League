// Package assets holds the tool's icon: a replay arrow around an upload
// arrow on a violet tile, grey while paused and with a red dot after failed
// uploads. gen.go draws it; the program file (cmd/auto-replay-upload/winres)
// and the interface (internal/web/ui/icon.svg) use the same drawing.
package assets

//go:generate go run gen.go

import _ "embed"

// Icon is the icon as a 64×64 PNG, for notifications.
//
//go:embed icon.png
var Icon []byte
