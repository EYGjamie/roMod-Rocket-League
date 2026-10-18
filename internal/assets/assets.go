// Package assets holds the tool's icon: an arrow pointing up, grey while
// paused and with a red dot after failed uploads.
package assets

import _ "embed"

// Icon is the icon as a 64×64 PNG, for notifications.
//
//go:embed icon.png
var Icon []byte
