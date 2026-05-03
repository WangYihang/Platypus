package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// handleInstall executes the install protocol over an open
// STREAM_TYPE_PLUGIN_MGMT stream. The high-level flow:
//
//  1. RECEIVE: read manifest + wasm + signature bytes (inline) or
//     fetch them via HTTPS (URL source — Phase 2). See install_receive.go.
//  2. VERIFY_SHA: compare the wasm sha256 against the URL source's
//     declared digest (skipped for inline; the trust there is the
//     server-side mTLS to the operator's session).
//  3. VERIFY_SIG: minisign verification of the wasm against the
//     publisher's public key.
//  4. EXTRACT: stage manifest + wasm + sig under a fresh
//     installed/<id>/<version>/ directory (atomic rename from a tmp
//     dir, so a crash mid-write doesn't leave a partial install).
//     See install_persist.go.
//  5. LOAD: instantiate the plugin via extism so any malformed wasm
//     is caught before the catalog is updated.
//  6. INSTALLED: catalog upsert + in-memory registry insert; final
//     PHASE_INSTALLED frame.
//
// On any error before the catalog upsert, partial filesystem state is
// removed and a PHASE_FAILED frame is emitted. The installed plugin is
// only visible to Invoke() once the catalog upsert succeeds, so a
// failed install never leaves the runtime with a half-loaded plugin.
func (r *Registry) handleInstall(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.PluginInstallRequest) error {
	if err := validateInstallRequest(req); err != nil {
		return failInstall(stream, "invalid_request", err)
	}

	pk, keyID, err := LoadPublicKeyFromBytes(req.GetPublisherPubkey())
	if err != nil {
		return failInstall(stream, "publisher_key_invalid", err)
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_RECEIVE})

	manifestBytes, wasmBytes, sigBytes, err := receiveSource(ctx, stream, req)
	if err != nil {
		return failInstall(stream, "receive_failed", err)
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{
		Phase:      v2pb.PluginInstallProgress_PHASE_VERIFY_SHA,
		BytesDone:  uint64(len(wasmBytes)),
		BytesTotal: uint64(len(wasmBytes)),
	})
	if u := req.GetUrl(); u != nil && len(u.GetWasmSha256()) > 0 {
		if err := verifySha256(wasmBytes, u.GetWasmSha256()); err != nil {
			return failInstall(stream, "sha256_mismatch", err)
		}
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_VERIFY_SIG})

	sig, err := LoadSignatureFromBytes(sigBytes)
	if err != nil {
		return failInstall(stream, "signature_decode_failed", err)
	}
	if err := VerifyWasm(pk, wasmBytes, sig); err != nil {
		return failInstall(stream, "signature_mismatch", err)
	}

	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return failInstall(stream, "manifest_invalid", err)
	}
	if manifest.ID != req.GetPluginId() {
		return failInstall(stream, "manifest_id_mismatch",
			fmt.Errorf("manifest id=%q != install request id=%q", manifest.ID, req.GetPluginId()))
	}
	if manifest.Version != req.GetVersion() {
		return failInstall(stream, "manifest_version_mismatch",
			fmt.Errorf("manifest version=%q != install request version=%q", manifest.Version, req.GetVersion()))
	}
	if err := manifest.ValidateGranted(req.GetGrantedCapabilities()); err != nil {
		return failInstall(stream, "capability_overgrant", err)
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_EXTRACT})

	if err := r.persistInstall(req, manifest, manifestBytes, wasmBytes, sigBytes); err != nil {
		return failInstall(stream, "extract_failed", err)
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_LOAD})

	entry := CatalogEntry{
		ID:                  req.GetPluginId(),
		Version:             req.GetVersion(),
		Name:                manifest.Name,
		Author:              manifest.Author.Name,
		Enabled:             true,
		GrantedCapabilities: req.GetGrantedCapabilities(),
		InstalledAt:         time.Now(),
		PublisherKeyID:      keyID,
	}
	if u := req.GetUrl(); u != nil {
		entry.SourceURL = u.GetWasmUrl()
	}

	loaded, err := r.hotLoad(ctx, entry, manifest, pk)
	if err != nil {
		// Remove the freshly-extracted dir to avoid leaving an
		// orphaned, never-loaded plugin on disk.
		_ = os.RemoveAll(r.paths.VersionDir(entry.ID, entry.Version))
		return failInstall(stream, "load_failed", err)
	}

	if err := r.catalog.Upsert(entry); err != nil {
		loaded.close(ctx)
		_ = os.RemoveAll(r.paths.VersionDir(entry.ID, entry.Version))
		return failInstall(stream, "catalog_write_failed", err)
	}
	r.mu.Lock()
	if existing, ok := r.plugins[entry.ID]; ok {
		// An in-place upgrade closes the prior version's runtime so the
		// new one takes over without a stale extism.Plugin lingering.
		existing.close(ctx)
	}
	r.plugins[entry.ID] = loaded
	r.mu.Unlock()

	log.L.Info("plugin.install.ok",
		"plugin_id", entry.ID,
		"version", entry.Version,
		"publisher_key_id", entry.PublisherKeyID,
		"granted_capabilities", entry.GrantedCapabilities,
		"actor", req.GetActor(),
	)

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_INSTALLED})
	return nil
}

// failInstall logs, emits a terminal PHASE_FAILED frame, and returns
// the underlying error. Centralised so every error path looks the same
// in the logs and on the wire.
func failInstall(stream io.Writer, code string, err error) error {
	log.L.Warn("plugin.install.failed", "code", code, "error", err.Error())
	emitInstallProgress(stream, &v2pb.PluginInstallProgress{
		Phase:        v2pb.PluginInstallProgress_PHASE_FAILED,
		ErrorCode:    code,
		ErrorMessage: err.Error(),
	})
	return fmt.Errorf("%s: %w", code, err)
}
