// Package system bootstraps the agent-bundled "system plugins" — the
// set of capability-elevated plugins shipped inside the platypus-agent
// binary itself. These are the plugin-ified replacements for the
// historical hardcoded RPC handlers (sysinfo, exec, listdir, etc.):
// the agent binary stays small and feature-frozen at its plugin
// framework, while the actual functionality is delivered as signed
// .wasm modules baked in at build time.
//
// On every agent boot, EnsureInstalled walks the embedded FS, verifies
// each plugin against the system signing pubkey, and installs any
// that are missing or out-of-version in the live Registry. Result:
// the operator never sees a fresh agent that can't `exec` or
// `list_dir` because someone forgot to install the plugin — boot is
// the install moment.
//
// Layout under embedded/:
//
//	embedded/
//	  publisher.pub                       minisign-format system signing pubkey
//	  <plugin_id>/<version>/
//	    plugin.yaml
//	    <entry>.wasm
//	    <entry>.wasm.minisig
//
// publisher.pub lives at the FS root so the system signing key
// rotates with the agent binary itself (one key for every system
// plugin in this build).
package system

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/log"
)

// publisherFile is the well-known location of the system signing
// pubkey inside the embedded FS. Centralised so both EnsureInstalled
// and tests reference one constant.
const publisherFile = "publisher.pub"

// EnsureInstalled walks `embFS` (typically the embed.FS produced by
// //go:embed in the caller, but any fs.FS works for testing) and
// installs each <plugin_id>/<version>/ tree it finds via
// reg.InstallFromBytes. Plugins already present at the same id +
// version are left alone; mismatched versions trigger an in-place
// re-install (the runtime's hot-load path tears down the old
// instance).
//
// Errors during one plugin's install are logged and skipped so a
// single broken bundled plugin can't prevent the agent from booting.
// The caller decides whether to treat the per-plugin error count as
// fatal (the production agent does not — see cmd/platypus-agent).
func EnsureInstalled(ctx context.Context, reg *plugin.Registry, embFS fs.FS) Result {
	res := Result{}

	pubBytes, err := fs.ReadFile(embFS, publisherFile)
	if err != nil {
		res.SetupError = fmt.Errorf("system: read %s: %w", publisherFile, err)
		log.L.Error("plugin.system.no_publisher", "error", res.SetupError.Error())
		return res
	}

	bundles, err := discoverBundles(embFS)
	if err != nil {
		res.SetupError = err
		log.L.Error("plugin.system.discover", "error", err.Error())
		return res
	}

	for _, b := range bundles {
		log.L.Info("plugin.system.ensure", "plugin_id", b.ID, "version", b.Version)
		err := installOne(ctx, reg, embFS, b, pubBytes)
		switch {
		case err == nil:
			res.Installed = append(res.Installed, b)
		case errors.Is(err, errAlreadyInstalled):
			res.Skipped = append(res.Skipped, b)
		default:
			res.Failed = append(res.Failed, FailedBundle{Bundle: b, Err: err})
			log.L.Warn("plugin.system.install_failed",
				"plugin_id", b.ID, "version", b.Version, "error", err.Error())
		}
	}
	return res
}

// Bundle identifies one plugin staged in the embedded FS.
type Bundle struct {
	ID      string
	Version string
}

// FailedBundle is one Bundle that didn't install, plus the reason.
type FailedBundle struct {
	Bundle
	Err error
}

// Result summarises one EnsureInstalled call. SetupError is set when
// EnsureInstalled couldn't even start (e.g. publisher.pub missing);
// in that case the three slice fields are empty. Otherwise SetupError
// is nil and the slices partition the discovered bundle set.
type Result struct {
	SetupError error
	Installed  []Bundle
	Skipped    []Bundle
	Failed     []FailedBundle
}

// errAlreadyInstalled is the sentinel for "this id+version already
// matches what's in the catalog; no work needed". Wrapped so callers
// can use errors.Is.
var errAlreadyInstalled = errors.New("system: plugin already installed at this version")

// discoverBundles walks embFS for two-level <id>/<version>/ dirs.
// Returns a sorted list (id then version) so both the install order
// and the Result slices are deterministic.
func discoverBundles(embFS fs.FS) ([]Bundle, error) {
	var out []Bundle

	ids, err := fs.ReadDir(embFS, ".")
	if err != nil {
		return nil, fmt.Errorf("system: read root: %w", err)
	}
	for _, d := range ids {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		versions, err := fs.ReadDir(embFS, id)
		if err != nil {
			return nil, fmt.Errorf("system: read %s: %w", id, err)
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			out = append(out, Bundle{ID: id, Version: v.Name()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// installOne reads the three artefacts for one bundle off the
// embedded FS and hands them to reg.InstallFromBytes. Returns
// errAlreadyInstalled when the catalog already has the same
// id+version pair.
func installOne(ctx context.Context, reg *plugin.Registry, embFS fs.FS, b Bundle, pubBytes []byte) error {
	if reg.HasInstalledVersion(b.ID, b.Version) {
		return errAlreadyInstalled
	}

	manifestBytes, wasmBytes, sigBytes, err := readBundle(embFS, b)
	if err != nil {
		return err
	}
	manifest, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	// System plugins are granted every capability the manifest
	// requests — they're shipped by us, the operator implicitly trusts
	// them by trusting the agent build. The capability dialog only
	// applies to user-installed third-party plugins.
	granted := []string{}
	for _, c := range manifest.DeclaredCapabilities() {
		granted = append(granted, string(c))
	}

	return reg.InstallFromBytes(ctx, plugin.InstallParams{
		PluginID:            b.ID,
		Version:             b.Version,
		PublisherPubkey:     pubBytes,
		Manifest:            manifestBytes,
		Wasm:                wasmBytes,
		Signature:           sigBytes,
		GrantedCapabilities: granted,
		Actor:               "system:bundle",
		System:              true,
	}, nil)
}

// readBundle pulls the three artefacts (manifest + wasm + sig) off the
// embedded FS for one bundle. The .wasm filename comes from the
// manifest's runtime.entry — we parse the manifest first to learn it,
// then go back for the wasm + sig. A small two-step but keeps the
// embedded layout flexible (wasm filename isn't fixed).
func readBundle(embFS fs.FS, b Bundle) (manifest, wasm, sig []byte, err error) {
	dir := path.Join(b.ID, b.Version)
	manifest, err = fs.ReadFile(embFS, path.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := plugin.ParseManifest(manifest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse manifest: %w", err)
	}
	if strings.ContainsAny(m.Runtime.Entry, "/\\") {
		return nil, nil, nil, fmt.Errorf("manifest entry %q must be a plain filename", m.Runtime.Entry)
	}
	wasm, err = fs.ReadFile(embFS, path.Join(dir, m.Runtime.Entry))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read wasm: %w", err)
	}
	sig, err = fs.ReadFile(embFS, path.Join(dir, m.Runtime.Entry+".minisig"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read sig: %w", err)
	}
	return manifest, wasm, sig, nil
}
