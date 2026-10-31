// Package assets holds the tool's icon: a download arrow on a violet
// triangle, grey while paused and with a red dot after failed
// uploads. gen.go draws it; the program file (cmd/romod/winres)
// and the interface (internal/web/ui/icon.svg) use the same drawing.
// It also holds the rank logos of the lobby popup.
package assets

//go:generate go run gen.go

import "embed"

// Icon is the icon as a 64×64 PNG, for notifications.
//
//go:embed icon.png
var Icon []byte

// Ranks holds the rank logos as 136×136 PNGs, four times the size the
// lobby popup draws them: ranks/0.png is Unranked, ranks/1.png Bronze I up
// to ranks/22.png Supersonic Legend. They are the ones tracker.gg shows
// (trackercdn.com/cdn/tracker.gg/rocket-league/ranks/s4-<tier>.png, the
// Grand Champions s15rank<tier>.png), made smaller once.
//
//go:embed ranks/*.png
var Ranks embed.FS
