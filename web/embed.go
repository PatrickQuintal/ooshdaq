// Package web exposes the browser application as an immutable embedded
// filesystem for the single-binary server.
package web

import (
	"embed"
	"io/fs"
)

// content includes only reviewed browser assets. Dotfiles and this Go source
// are intentionally outside the patterns.
//
//go:embed *.html *.css *.js assets styles js
var content embed.FS

// Files returns the embedded browser application rooted at index.html.
func Files() fs.FS {
	return content
}
