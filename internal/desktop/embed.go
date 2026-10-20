package desktop

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var embeddedWebDist embed.FS

func webDistFS() (fs.FS, error) {
	return fs.Sub(embeddedWebDist, "web/dist")
}
