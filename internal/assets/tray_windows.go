package assets

import _ "embed"

// The tray icons, as ICO files with all sizes Windows asks for.
var (
	//go:embed icon.ico
	Tray []byte
	//go:embed icon-paused.ico
	TrayPaused []byte
	//go:embed icon-error.ico
	TrayError []byte
)
