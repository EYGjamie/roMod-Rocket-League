// Package web holds the files of the interface, shown in the overlay and
// in the browser alike.
package web

import "embed"

//go:embed ui
var UI embed.FS
