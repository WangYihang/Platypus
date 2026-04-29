package artifact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStore is a Store backed by a directory on the server's
// filesystem. The directory layout matches the bucket layout
// documented at the top of store.go:
//
//	<root>/manifest/<channel>.json
//	<root>/manifest/<channel>.json.sig
//	<root>/artifacts/<version>/<os>/<arch>/platypus-agent[.exe]
//
// Operators populate <root> by rsyncing release artifacts from the
// CI pipeline; the server never writes here. Single-server fleets
// (the typical Platypus deployment) get the entire distribution
// pipeline without a MinIO sidecar or AWS account.
//
// Larger deployments that need multi-server / CDN / regional egress
// keep using the S3Store implementation.
type LocalStore struct {
	root string
}

// NewLocalStore returns a Store rooted at the given directory. The
// directory must exist; missing-on-startup is treated as a
// configuration error (rather than auto-mkdir) because empty roots
// produce 404s on every request and we want the operator to notice
// at boot, not at first agent self-upgrade.
func NewLocalStore(root string) (*LocalStore, error) {
	if root == "" {
		return nil, fmt.Errorf("artifact: local-store root is empty")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("artifact: stat local-store root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact: local-store root %q is not a directory", root)
	}
	return &LocalStore{root: root}, nil
}

// Prefix is empty for local stores — the whole directory is the
// distribution root, no shared-bucket prefix needed.
func (l *LocalStore) Prefix() string { return "" }

// fullPath joins the configured root with a caller-supplied key,
// rejecting `..` traversal so a poisoned manifest can't escape the
// release directory and serve, say, /etc/passwd.
func (l *LocalStore) fullPath(key string) (string, error) {
	cleaned := filepath.Clean("/" + key) // "/" anchor blocks ".." escape
	abs := filepath.Join(l.root, cleaned)
	rel, err := filepath.Rel(l.root, abs)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || hasParentTraversal(rel) {
		return "", fmt.Errorf("artifact: key %q escapes local-store root", key)
	}
	return abs, nil
}

func hasParentTraversal(rel string) bool {
	for _, part := range filepath.SplitList(rel) {
		if part == ".." {
			return true
		}
	}
	// SplitList only handles PATH-separator strings; for filesystem
	// paths we also need to walk separators directly.
	return splitContainsParent(rel)
}

func splitContainsParent(rel string) bool {
	for {
		dir, leaf := filepath.Split(rel)
		if leaf == ".." {
			return true
		}
		if dir == "" || dir == rel {
			return false
		}
		rel = filepath.Clean(dir)
	}
}

// GetObject reads the whole file into memory. Used for the manifest
// + signature only, both of which are well under a megabyte.
func (l *LocalStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	p, err := l.fullPath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("artifact: read %s: %w", key, err)
	}
	return data, nil
}

// GetObjectReader opens the file for streaming. Caller closes.
// Content type is left empty; the Distributor's HTTP handler runs
// http.DetectContentType on the first 512 bytes anyway, which is a
// truer signal than the filesystem's (often missing) xattr metadata.
func (l *LocalStore) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	p, err := l.fullPath(key)
	if err != nil {
		return nil, 0, "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, 0, "", fmt.Errorf("artifact: open %s: %w", key, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, "", fmt.Errorf("artifact: stat %s: %w", key, err)
	}
	return f, info.Size(), "", nil
}
