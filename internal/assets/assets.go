// Package assets provides the frontend files embedded in the desktop binary.
package assets

import (
	"embed"
	"io/fs"
)

// Embed the package directory so clean checkouts compile before the frontend is built.
// Production build tasks populate the ignored frontend directory before Go compilation.
//
//go:embed *
var files embed.FS

// Frontend returns the embedded frontend distribution root.
func Frontend() (fs.FS, error) {
	return fs.Sub(files, "frontend")
}
