package system

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// Source is the resolved system-plugin tree and a human-readable
// origin label. Origin shows up in boot logs ("data-dir" vs
// "embedded") so operators can tell at a glance which set of bundles
// + signing key the agent is about to verify against.
type Source struct {
	FS     fs.FS
	Origin string
}

// OverrideSubdir is the basename inside the agent's data directory
// where a development / dev-compose / fleet-rollout publisher can
// stage a fresh system-plugin tree. Layout under it mirrors the embed
// FS exactly: publisher.pub at the root, then <plugin_id>/<version>/
// subdirs each holding plugin.yaml + <entry>.wasm + .minisig.
//
// Exported (vs the lowercase publisherFile) because external tooling
// — the dev publisher script, e2e fixtures, integration test helpers —
// needs to construct paths under it.
const OverrideSubdir = "system-plugins"

// ResolveSource decides which system-plugin tree to use this boot.
// Resolution order:
//
//  1. <dataDir>/system-plugins/ — when present and well-formed (a
//     publisher.pub at the root). Used by the dev compose stack:
//     the agent-publisher sidecar stages a freshly-built +
//     freshly-signed bundle here so a `docker compose up` lands
//     working file-read / process-open / etc. capabilities without
//     rebuilding the agent binary.
//  2. The embedded FS (//go:embed all:embedded). Used by every
//     production deployment and by tests that haven't seeded the
//     override.
//
// dataDir == "" forces the embedded path (caller didn't resolve a
// data dir yet); a missing or malformed override silently falls back
// (caller logs Origin so the choice is auditable).
//
// "Well-formed" check is intentionally lightweight: existence of
// publisher.pub. Catalogue / signature errors surface inside
// EnsureInstalled, so a bad override produces a normal Result.Failed
// list (loud but per-bundle), not a silent demote-to-embed which
// would hide install failures in the override tree.
func ResolveSource(dataDir string) (Source, error) {
	if dataDir != "" {
		overrideRoot := filepath.Join(dataDir, OverrideSubdir)
		if hasPublisher(overrideRoot) {
			return Source{FS: os.DirFS(overrideRoot), Origin: "data-dir"}, nil
		}
	}
	sub, err := EmbeddedFS()
	if err != nil {
		return Source{}, err
	}
	return Source{FS: sub, Origin: "embedded"}, nil
}

// hasPublisher returns true when overrideRoot/publisher.pub exists
// and is a regular file. We deliberately don't validate the key
// material here: the EnsureInstalled path already loads + uses it,
// and a corrupt key produces a clear per-bundle "signature_mismatch"
// error there. Keeping the check shallow makes it cheap to call on
// every boot.
func hasPublisher(overrideRoot string) bool {
	st, err := os.Stat(filepath.Join(overrideRoot, publisherFile))
	if err != nil {
		// Treat any stat failure (missing, permission denied, EIO) as
		// "no override" — the embed FS is the safer fallback.
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return false
	}
	return st.Mode().IsRegular()
}
