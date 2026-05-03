package plugin

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// validateInstallRequest is the cheap up-front sanity check. Caught
// here, the failure shows up before any bytes are read off the wire.
func validateInstallRequest(req *v2pb.PluginInstallRequest) error {
	if !idRegexp.MatchString(req.GetPluginId()) {
		return fmt.Errorf("plugin_id=%q is not a valid reverse-DNS id", req.GetPluginId())
	}
	if !versionRegexp.MatchString(req.GetVersion()) {
		return fmt.Errorf("version=%q is not strict semver", req.GetVersion())
	}
	if len(req.GetPublisherPubkey()) == 0 {
		return errors.New("publisher_pubkey is required")
	}
	if req.GetSource() == nil {
		return errors.New("source is required (one of inline / url)")
	}
	if u := req.GetUrl(); u != nil {
		if u.GetWasmUrl() == "" || u.GetSignatureUrl() == "" || u.GetManifestUrl() == "" {
			return errors.New("url source requires wasm_url, signature_url, manifest_url")
		}
		if len(u.GetWasmSha256()) != sha256.Size {
			return fmt.Errorf("url.wasm_sha256 must be %d bytes", sha256.Size)
		}
	}
	return nil
}

// receiveSource reads the manifest, wasm, and signature bytes off the
// stream (inline source) or fetches them via HTTPS (URL source). For
// MVP the URL path returns an explicit "not implemented" so the
// operator UI can render that gracefully without crashing the agent.
func receiveSource(_ context.Context, stream io.Reader, req *v2pb.PluginInstallRequest) (manifest, wasm, sig []byte, err error) {
	switch s := req.GetSource().(type) {
	case *v2pb.PluginInstallRequest_Inline:
		return readInlineChunks(stream, s.Inline.GetWasmSizeBytes())
	case *v2pb.PluginInstallRequest_Url:
		return nil, nil, nil, errors.New("url source not implemented in MVP; use inline")
	default:
		return nil, nil, nil, errors.New("source variant unrecognised")
	}
}

// readInlineChunks consumes the agreed three-segment stream:
// MANIFEST chunks, then WASM chunks, then SIGNATURE chunks. Each
// segment is bounded by its own `last=true` frame; the segments must
// arrive in this order. wasmExpected is the size declared in the
// install request so we can refuse a wildly oversized stream early.
func readInlineChunks(stream io.Reader, wasmExpected uint64) (manifest, wasm, sig []byte, err error) {
	manifest, err = readChunkSegment(stream, v2pb.PluginInstallChunk_KIND_MANIFEST, 1*1024*1024)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("manifest: %w", err)
	}
	maxWasm := uint64(64 * 1024 * 1024)
	if wasmExpected > 0 && wasmExpected > maxWasm {
		maxWasm = wasmExpected
	}
	wasm, err = readChunkSegment(stream, v2pb.PluginInstallChunk_KIND_WASM, maxWasm)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("wasm: %w", err)
	}
	if wasmExpected > 0 && uint64(len(wasm)) != wasmExpected {
		return nil, nil, nil, fmt.Errorf("wasm: declared size=%d, got %d", wasmExpected, len(wasm))
	}
	sig, err = readChunkSegment(stream, v2pb.PluginInstallChunk_KIND_SIGNATURE, 8*1024)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("signature: %w", err)
	}
	return manifest, wasm, sig, nil
}

// readChunkSegment accumulates frames of one kind until last=true.
// Refuses early if a frame announces a different kind, if the
// accumulated size exceeds maxBytes, or if the stream ends before a
// last frame.
func readChunkSegment(stream io.Reader, expected v2pb.PluginInstallChunk_Kind, maxBytes uint64) ([]byte, error) {
	var buf []byte
	for {
		c, err := readChunk(stream)
		if err != nil {
			return nil, err
		}
		if c.GetKind() != expected {
			return nil, fmt.Errorf("expected kind=%s, got %s", expected, c.GetKind())
		}
		buf = append(buf, c.GetData()...)
		if uint64(len(buf)) > maxBytes {
			return nil, fmt.Errorf("size cap %d exceeded", maxBytes)
		}
		if c.GetLast() {
			return buf, nil
		}
	}
}

// verifySha256 cheaply checks the wasm against the declared digest.
// Used for URL installs as a length / mirror-corruption guard before
// the more expensive minisign verification.
func verifySha256(data []byte, want []byte) error {
	if len(want) != sha256.Size {
		return fmt.Errorf("expected %d-byte digest, got %d", sha256.Size, len(want))
	}
	got := sha256.Sum256(data)
	for i := 0; i < sha256.Size; i++ {
		if got[i] != want[i] {
			return errors.New("sha256 digest mismatch")
		}
	}
	return nil
}
