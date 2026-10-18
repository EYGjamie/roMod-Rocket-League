//go:build !windows && !linux && !(darwin && cgo)

package desktop

import (
	"context"
	"log"
	"runtime"
)

// HasTray reports that the tool shows no icon.
const HasTray = false

// Run waits for ctx: there is no icon without the tray library, which needs
// cgo on macOS.
func (t Tray) Run(ctx context.Context) {
	if runtime.GOOS == "darwin" {
		log.Print("Ohne cgo gebaut, deshalb kein Symbol in der Menüleiste")
	}
	<-ctx.Done()
}

func SetTrayState(paused, failed bool) {}
