// Package sysplugins owns the system-plugin bundles the server binary
// ships with and the resolver that picks between an operator-staged
// override (<data-dir>/system-plugins/) and the embedded prebuilt
// tree.
//
// The bundles themselves are produced by `go run
// ./hack/stage_system_plugins` from the rust artefacts under
// example/plugins/system/; that helper writes
//
//	embedded/system-plugins/
//	  publisher.pub
//	  <plugin-id>/<version>/{plugin.yaml,*.wasm,*.minisig}
//
// which we go:embed below. The reconciler in internal/api consumes
// whatever fs.FS Resolve returns; it doesn't know (and doesn't need
// to know) whether the bytes came from disk or from the binary.
package sysplugins

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:embedded/system-plugins
var fsRoot embed.FS

// PrebuiltFS returns the system-plugins/ tree shipped with the server
// binary, rooted such that fs.ReadFile(returned, "publisher.pub")
// works without prefixing.
func PrebuiltFS() fs.FS {
	sub, err := fs.Sub(fsRoot, "embedded/system-plugins")
	if err != nil {
		// Embed declarations are evaluated at compile time; a wrong
		// path would have failed the build, so this is unreachable
		// outside a go:embed regression. Panic to fail loudly rather
		// than serve an empty catalog.
		panic("sysplugins: embedded tree malformed: " + err.Error())
	}
	return sub
}

// Resolve picks the active system-plugins source. Operators who want
// to override the binary's bundles (e.g. air-gapped environments
// vendoring a custom signing key) drop their tree under
// <data-dir>/system-plugins/ and that wins.
//
// Detection: we look for publisher.pub at the disk root. An empty
// (mkdir-only) system-plugins/ directory does NOT count — a fresh
// install where a playbook ran `mkdir -p data-dir/system-plugins`
// should still get the embedded plugins, not an empty catalog with
// silent reconcile failures.
//
// dataDir == "" disables the disk override entirely; tests use this.
func Resolve(dataDir string) fs.FS {
	if dataDir != "" {
		probe := filepath.Join(dataDir, "system-plugins", "publisher.pub")
		if _, err := os.Stat(probe); err == nil {
			return os.DirFS(filepath.Join(dataDir, "system-plugins"))
		} else if !errors.Is(err, os.ErrNotExist) {
			// Permission errors and similar fall through to the
			// embedded tree; the operator can read the disk-side
			// log line via the surrounding handler if they care
			// to investigate.
		}
	}
	return PrebuiltFS()
}
