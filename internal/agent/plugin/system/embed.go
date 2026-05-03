package system

import (
	"embed"
	"io/fs"
)

// DefaultFS is the production embedded plugin tree. Compiled into the
// platypus-agent binary by `//go:embed all:embedded`. EnsureInstalled
// walks this on every agent boot.
//
// The pattern uses `all:` so dotfiles (none today, but a future
// maintainer who adds a `.gitkeep` under embedded/ won't accidentally
// exclude it) are included.
//
// Empty embedded/ trees are valid — EnsureInstalled returns
// SetupError pointing at the missing publisher.pub. The agent's
// main.go treats SetupError as non-fatal: a build with no system
// plugins (today's state) boots cleanly with zero installed system
// plugins, and the operator can still install user plugins over REST.
//
//go:embed all:embedded
var DefaultFS embed.FS

// EmbeddedFS returns the bundle tree without the top-level "embedded/"
// directory in the path, so callers see <plugin_id>/<version>/...
// directly. Equivalent to fs.Sub(DefaultFS, "embedded") with the error
// path collapsed at the call site.
func EmbeddedFS() (fs.FS, error) {
	return fs.Sub(DefaultFS, "embedded")
}
