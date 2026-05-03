package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jedisct1/go-minisign"
)

// persistInstallBytes writes the three artefacts atomically into
// installed/<id>/<version>/. Any failure rewinds the partial dir so a
// failed install never leaves a stray .tmp tree behind. Operates on
// already-loaded bytes so it's reusable from both the streaming
// install path and the system-plugin bootstrap.
func (r *Registry) persistInstallBytes(pluginID, version string, m *Manifest, manifestBytes, wasmBytes, sigBytes []byte) error {
	dir := r.paths.VersionDir(pluginID, version)
	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clean tmp: %w", err)
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return fmt.Errorf("mkdir tmp: %w", err)
	}
	files := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{filepath.Join(tmp, "plugin.yaml"), manifestBytes, 0o600},
		{filepath.Join(tmp, m.Runtime.Entry), wasmBytes, 0o600},
		{filepath.Join(tmp, m.Runtime.Entry+".minisig"), sigBytes, 0o600},
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, f.data, f.mode); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}
	// Replace any prior version dir for this exact version. Different
	// versions live in sibling dirs and are pruned on uninstall.
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("remove prior: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("mkdir parent: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// hotLoad builds a *loaded for the freshly-installed plugin without
// writing to the catalog yet. Returning the *loaded lets the caller
// decide whether to commit (catalog upsert + registry insert) or
// rewind (close + remove on-disk state).
func (r *Registry) hotLoad(ctx context.Context, e CatalogEntry, m *Manifest, pk minisign.PublicKey) (*loaded, error) {
	stateDir := r.paths.StateDir(e.ID)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state: %w", err)
	}
	wasmPath := r.paths.WasmFile(e.ID, e.Version, m.Runtime.Entry)
	sigPath := r.paths.SignatureFile(e.ID, e.Version, m.Runtime.Entry)

	opts := r.options
	opts.fillDefaults()

	granted := map[CapabilityID]bool{CapLog: true}
	for _, g := range e.GrantedCapabilities {
		granted[CapabilityID(g)] = true
	}
	l := &loaded{
		id: e.ID, manifest: m, entry: e, pubKey: pk,
		stateDir: stateDir, wasmPath: wasmPath, sigPath: sigPath,
		logs: newLogBuffer(opts.LogBufferLines),
	}
	emptyCorr := ""
	l.currentCorr.Store(&emptyCorr)
	l.pctx = &pluginCtx{
		id: e.ID, manifest: m, granted: granted, stateDir: stateDir,
		logSink: l.logs, now: opts.Now,
		correlationID: func() string {
			v := l.currentCorr.Load()
			if v == nil {
				return ""
			}
			return *v
		},
		maxFileReadSize: opts.MaxFileReadBytes,
		maxKVValueSize:  opts.MaxKVValueBytes,
		streams:         newStreamRegistry(),
	}
	// Smoke-instantiate so a malformed wasm is caught here, not on
	// the first invocation.
	if _, err := l.instanceOf(ctx); err != nil {
		return nil, err
	}
	return l, nil
}
