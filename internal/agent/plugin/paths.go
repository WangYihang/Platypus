// Package plugin implements the agent-side plugin runtime: discovery,
// signature verification, sandboxed execution via Extism (atop wazero),
// capability-checked host functions, and the STREAM_TYPE_PLUGIN_MGMT
// install/uninstall/list/enable/get_logs handler.
//
// On-disk layout under <identityDir>/plugins/:
//
//	plugins/
//	  catalog.json                        installed-plugin state (enabled, granted caps, …)
//	  publishers/<key_id>.pub             trusted minisign pubkeys
//	  installed/<plugin_id>/<version>/
//	      plugin.yaml                     manifest
//	      <entry>.wasm                    binary
//	      <entry>.wasm.minisig            detached signature
//	      state/                          host_kv_* backing store, mode 0700
//	  quarantine/<plugin_id>/             binaries that failed verification
//
// All paths are anchored under the agent's identity dir so test isolation
// (PLATYPUS_IDENTITY_DIR) keeps working unchanged.
package plugin

import (
	"path/filepath"
)

// Paths centralises the per-runtime directory layout. Construct with
// NewPaths(identityRoot) — identityRoot is the same value
// agent.ResolveIdentityDir returns, NOT the per-CA subdirectory.
// Plugins live above the per-CA layer because a single host may serve
// multiple projects but operators expect plugins to be a property of
// the host, not of the enrollment.
type Paths struct {
	root string // <identityRoot>/plugins
}

// NewPaths returns a Paths rooted at <identityRoot>/plugins. Callers
// should pass the value from agent.ResolveIdentityDir; the constructor
// does no I/O so it's safe to build before MkdirAll.
func NewPaths(identityRoot string) Paths {
	return Paths{root: filepath.Join(identityRoot, "plugins")}
}

// Root is the plugin tree root.
func (p Paths) Root() string { return p.root }

// CatalogFile is the JSON file listing installed plugins + their state.
func (p Paths) CatalogFile() string { return filepath.Join(p.root, "catalog.json") }

// PublishersDir holds trusted minisign pubkeys, one .pub file per key.
func (p Paths) PublishersDir() string { return filepath.Join(p.root, "publishers") }

// PublisherKeyFile is the minisign public key for the given key id.
func (p Paths) PublisherKeyFile(keyID string) string {
	return filepath.Join(p.PublishersDir(), keyID+".pub")
}

// InstalledDir is the parent of all installed plugin versions.
func (p Paths) InstalledDir() string { return filepath.Join(p.root, "installed") }

// PluginDir is <root>/installed/<plugin_id>/. A plugin id may have
// multiple version subdirs while an upgrade is staged.
func (p Paths) PluginDir(pluginID string) string {
	return filepath.Join(p.InstalledDir(), pluginID)
}

// VersionDir is the per-version directory holding manifest + .wasm + sig.
func (p Paths) VersionDir(pluginID, version string) string {
	return filepath.Join(p.PluginDir(pluginID), version)
}

// ManifestFile, WasmFile, SignatureFile are the three artefacts dropped
// into VersionDir at install time. Names are fixed so the loader can
// find them without consulting catalog.json.
func (p Paths) ManifestFile(pluginID, version string) string {
	return filepath.Join(p.VersionDir(pluginID, version), "plugin.yaml")
}
func (p Paths) WasmFile(pluginID, version, entry string) string {
	return filepath.Join(p.VersionDir(pluginID, version), entry)
}
func (p Paths) SignatureFile(pluginID, version, entry string) string {
	return filepath.Join(p.VersionDir(pluginID, version), entry+".minisig")
}

// StateDir is the per-plugin scratch dir for host_kv_* persistence.
// Lives under PluginDir (not VersionDir) so state survives upgrades;
// removed only when PluginUninstallRequest.purge_state is true.
func (p Paths) StateDir(pluginID string) string {
	return filepath.Join(p.PluginDir(pluginID), "state")
}

// QuarantineDir holds binaries that failed verification. Kept around
// for forensics; the agent never executes anything from here.
func (p Paths) QuarantineDir(pluginID string) string {
	return filepath.Join(p.root, "quarantine", pluginID)
}
