// Package artifact abstracts the object store that holds released agent
// binaries and the signed release manifest. The Distributor is a thin
// facade in front of this store — it never proxies bytes, only issues
// short-lived presigned URLs and forwards the (small) manifest.
package artifact

import (
	"context"
	"io"
)

// Manifest layout in the bucket.
//
//	{prefix}/manifest/{channel}.json
//	{prefix}/manifest/{channel}.json.sig
//	{prefix}/artifacts/{version}/{os}/{arch}/platypus-agent[.exe]
const (
	ManifestKeyFmt    = "manifest/%s.json"
	ManifestSigKeyFmt = "manifest/%s.json.sig"
)

// Store is the minimal surface the Distributor needs. A single
// implementation backed by S3/MinIO lives in s3.go; the interface exists
// so tests can drop in a fake.
type Store interface {
	// GetObject reads a whole object. Used for the manifest + sig, which
	// are small and need to be inspected before serving.
	GetObject(ctx context.Context, key string) ([]byte, error)

	// GetObjectReader returns a streaming reader for key, plus size and
	// content type. The caller must close the returned ReadCloser.
	// Used by the Distributor to proxy agent binary downloads.
	GetObjectReader(ctx context.Context, key string) (io.ReadCloser, int64, string, error)

	// Prefix returns the bucket-local prefix under which ManifestKeyFmt /
	// artifact keys are rooted. Handy for building full keys without
	// having to know the concrete backend's config.
	Prefix() string
}
