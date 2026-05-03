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
// STREAM_TYPE_PLUGIN_MGMT stream. The streaming-specific work
// (chunk reads + progress emission) lives here; the verify / extract
// / load / register pipeline lives in InstallFromBytes so non-stream
// callers — most importantly the system-plugin bootstrap — can reuse
// the exact same correctness guarantees without simulating a stream.
func (r *Registry) handleInstall(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.PluginInstallRequest) error {
	if err := validateInstallRequest(req); err != nil {
		return failInstall(stream, "invalid_request", err)
	}

	emitInstallProgress(stream, &v2pb.PluginInstallProgress{Phase: v2pb.PluginInstallProgress_PHASE_RECEIVE})

	manifestBytes, wasmBytes, sigBytes, err := receiveSource(ctx, stream, req)
	if err != nil {
		return failInstall(stream, "receive_failed", err)
	}

	params := InstallParams{
		PluginID:            req.GetPluginId(),
		Version:             req.GetVersion(),
		PublisherPubkey:     req.GetPublisherPubkey(),
		Manifest:            manifestBytes,
		Wasm:                wasmBytes,
		Signature:           sigBytes,
		GrantedCapabilities: req.GetGrantedCapabilities(),
		Actor:               req.GetActor(),
	}
	if u := req.GetUrl(); u != nil {
		params.SourceURL = u.GetWasmUrl()
		params.DigestSHA256 = u.GetWasmSha256()
	}

	emit := func(p *v2pb.PluginInstallProgress) { emitInstallProgress(stream, p) }
	if err := r.InstallFromBytes(ctx, params, emit); err != nil {
		// InstallFromBytes already emitted PHASE_FAILED via the
		// progress callback; just propagate the wrapped error.
		return err
	}
	return nil
}

// InstallParams carries the bytes the install pipeline needs after the
// receive phase. Used by handleInstall (after reading from the wire)
// and by the system-plugin bootstrap (which loads from embed.FS).
type InstallParams struct {
	PluginID            string
	Version             string
	PublisherPubkey     []byte // raw minisign .pub file contents
	Manifest            []byte
	Wasm                []byte
	Signature           []byte // raw .minisig file contents
	GrantedCapabilities []string
	Actor               string

	// SourceURL is recorded in the catalog for marketplace installs;
	// empty for inline / bundled.
	SourceURL string

	// DigestSHA256 (32 bytes) verifies the wasm before signature
	// check. Required for URL-source installs (defense in depth);
	// optional otherwise (the signature is the trust anchor).
	DigestSHA256 []byte

	// System marks the catalog entry as system-managed. System plugins
	// are auto-reinstalled on agent boot and refuse to uninstall via
	// REST.
	System bool
}

// InstallFromBytes runs the verify → extract → load → register
// pipeline against a fully-loaded byte set. progress is optional; pass
// nil to suppress per-phase frames (the system-plugin bootstrap path
// does this — it logs phase transitions itself rather than streaming
// them anywhere).
func (r *Registry) InstallFromBytes(ctx context.Context, params InstallParams, progress func(*v2pb.PluginInstallProgress)) error {
	if progress == nil {
		progress = func(*v2pb.PluginInstallProgress) {}
	}
	emit := func(p v2pb.PluginInstallProgress_Phase, code, msg string) {
		progress(&v2pb.PluginInstallProgress{Phase: p, ErrorCode: code, ErrorMessage: msg})
	}
	failBytes := func(code string, err error) error {
		log.L.Warn("plugin.install.failed", "code", code, "error", err.Error(),
			"plugin_id", params.PluginID, "version", params.Version)
		emit(v2pb.PluginInstallProgress_PHASE_FAILED, code, err.Error())
		return fmt.Errorf("%s: %w", code, err)
	}

	pk, keyID, err := LoadPublicKeyFromBytes(params.PublisherPubkey)
	if err != nil {
		return failBytes("publisher_key_invalid", err)
	}

	progress(&v2pb.PluginInstallProgress{
		Phase:      v2pb.PluginInstallProgress_PHASE_VERIFY_SHA,
		BytesDone:  uint64(len(params.Wasm)),
		BytesTotal: uint64(len(params.Wasm)),
	})
	if len(params.DigestSHA256) > 0 {
		if err := verifySha256(params.Wasm, params.DigestSHA256); err != nil {
			return failBytes("sha256_mismatch", err)
		}
	}

	emit(v2pb.PluginInstallProgress_PHASE_VERIFY_SIG, "", "")
	sig, err := LoadSignatureFromBytes(params.Signature)
	if err != nil {
		return failBytes("signature_decode_failed", err)
	}
	if err := VerifyWasm(pk, params.Wasm, sig); err != nil {
		return failBytes("signature_mismatch", err)
	}

	manifest, err := ParseManifest(params.Manifest)
	if err != nil {
		return failBytes("manifest_invalid", err)
	}
	if manifest.ID != params.PluginID {
		return failBytes("manifest_id_mismatch",
			fmt.Errorf("manifest id=%q != install request id=%q", manifest.ID, params.PluginID))
	}
	if manifest.Version != params.Version {
		return failBytes("manifest_version_mismatch",
			fmt.Errorf("manifest version=%q != install request version=%q", manifest.Version, params.Version))
	}
	if err := manifest.ValidateGranted(params.GrantedCapabilities); err != nil {
		return failBytes("capability_overgrant", err)
	}

	emit(v2pb.PluginInstallProgress_PHASE_EXTRACT, "", "")
	if err := r.persistInstallBytes(params.PluginID, params.Version, manifest,
		params.Manifest, params.Wasm, params.Signature); err != nil {
		return failBytes("extract_failed", err)
	}

	emit(v2pb.PluginInstallProgress_PHASE_LOAD, "", "")
	entry := CatalogEntry{
		ID:                  params.PluginID,
		Version:             params.Version,
		Name:                manifest.Name,
		Author:              manifest.Author.Name,
		Enabled:             true,
		GrantedCapabilities: params.GrantedCapabilities,
		InstalledAt:         time.Now(),
		PublisherKeyID:      keyID,
		SourceURL:           params.SourceURL,
		System:              params.System,
	}
	loaded, err := r.hotLoad(ctx, entry, manifest, pk)
	if err != nil {
		_ = os.RemoveAll(r.paths.VersionDir(entry.ID, entry.Version))
		return failBytes("load_failed", err)
	}

	if err := r.catalog.Upsert(entry); err != nil {
		loaded.close(ctx)
		_ = os.RemoveAll(r.paths.VersionDir(entry.ID, entry.Version))
		return failBytes("catalog_write_failed", err)
	}
	r.mu.Lock()
	if existing, ok := r.plugins[entry.ID]; ok {
		// In-place upgrade: tear down the prior wasm runtime so the
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
		"system", entry.System,
		"actor", params.Actor,
	)
	emit(v2pb.PluginInstallProgress_PHASE_INSTALLED, "", "")
	return nil
}

// failInstall is the stream-side error helper retained for
// handleInstall's pre-receive guards (validate / receive). Once the
// pipeline is in InstallFromBytes the failBytes closure inside that
// function takes over.
func failInstall(stream io.Writer, code string, err error) error {
	log.L.Warn("plugin.install.failed", "code", code, "error", err.Error())
	emitInstallProgress(stream, &v2pb.PluginInstallProgress{
		Phase:        v2pb.PluginInstallProgress_PHASE_FAILED,
		ErrorCode:    code,
		ErrorMessage: err.Error(),
	})
	return fmt.Errorf("%s: %w", code, err)
}
