package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// DistFS returns the filesystem rooted at the dist/ directory.
func DistFS() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
