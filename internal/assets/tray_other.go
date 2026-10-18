//go:build !windows

package assets

import _ "embed"

// The tray icons, as PNG files for macOS and Linux.
var (
	Tray = Icon
	//go:embed icon-paused.png
	TrayPaused []byte
	//go:embed icon-error.png
	TrayError []byte
)
