package plugin

// CapabilityID is the stable string a plugin uses to declare access to
// a host-function family. The set is fixed agent-side: a plugin cannot
// invent new capabilities. Keeping this list short and grep-able is
// deliberate — every value here corresponds to enforcement code in
// host_*.go.
type CapabilityID string

const (
	CapLog     CapabilityID = "log"      // host_log; granted implicitly to every plugin
	CapKV      CapabilityID = "kv"       // host_kv_get/put under plugin's own namespace
	CapSysInfo CapabilityID = "sysinfo"  // host_sysinfo: read-only host snapshot
	CapExec    CapabilityID = "exec"     // host_exec; requires Capabilities.Exec.Commands allowlist
	CapFSRead  CapabilityID = "fs.read"  // host_fs_read/listdir/stat; requires Capabilities.FSRead.Paths
	CapNetHTTP CapabilityID = "net.http" // host_http; requires Capabilities.NetHTTP.Hosts
)

// allCapabilities is the set the agent is willing to grant. Used by
// validation to reject manifests that ask for unknown capabilities.
var allCapabilities = map[CapabilityID]struct{}{
	CapLog: {}, CapKV: {}, CapSysInfo: {}, CapExec: {}, CapFSRead: {}, CapNetHTTP: {},
}

// Manifest is the plugin.yaml spec. See docs/plugins/AUTHORS.md for the
// authoring side. Validation lives in manifest_validate.go so the
// pure-data type definitions stay easy to skim.
type Manifest struct {
	APIVersion   int                  `yaml:"api_version"`
	ID           string               `yaml:"id"`
	Name         string               `yaml:"name"`
	Version      string               `yaml:"version"`
	Author       ManifestAuthor       `yaml:"author"`
	License      string               `yaml:"license"`
	Homepage     string               `yaml:"homepage"`
	Description  string               `yaml:"description"`
	Runtime      ManifestRuntime      `yaml:"runtime"`
	RPC          []ManifestRPC        `yaml:"rpc"`
	Capabilities ManifestCapabilities `yaml:"capabilities"`
	Resources    ManifestResources    `yaml:"resources"`
	Signature    ManifestSignature    `yaml:"signature"`
}

type ManifestAuthor struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

type ManifestRuntime struct {
	Type  string `yaml:"type"`  // "wasm"; only valid value in MVP
	Entry string `yaml:"entry"` // .wasm filename relative to manifest
	ABI   string `yaml:"abi"`   // "extism/1"
}

type ManifestRPC struct {
	Name            string         `yaml:"name"`             // wasm export name
	Request         ManifestSchema `yaml:"request"`
	Response        ManifestSchema `yaml:"response"`
	ProtoDescriptor string         `yaml:"proto_descriptor"` // optional FileDescriptorSet path
}

type ManifestSchema struct {
	Proto string `yaml:"proto"` // protobuf message name; informational
}

type ManifestCapabilities struct {
	Exec    *CapExecSpec    `yaml:"exec,omitempty"`
	FSRead  *CapFSReadSpec  `yaml:"fs.read,omitempty"`
	NetHTTP *CapNetHTTPSpec `yaml:"net.http,omitempty"`
	KV      bool            `yaml:"kv,omitempty"`
	SysInfo bool            `yaml:"sysinfo,omitempty"`
}

// CapExecSpec.Commands is the exact-path allowlist of executables the
// plugin is permitted to run. Wildcards are NOT supported by design —
// the operator should be able to read this list and know precisely
// what binaries the plugin can spawn.
type CapExecSpec struct {
	Commands []string `yaml:"commands"`
}

// CapFSReadSpec.Paths is the directory allowlist; reads are permitted
// only for paths that are equal to or descend from one of these. Symlink
// traversal is NOT followed across allowlist boundaries (resolved
// runtime-side in host_fs.go).
type CapFSReadSpec struct {
	Paths []string `yaml:"paths"`
}

// CapNetHTTPSpec.Hosts is the allowlist for outbound HTTP. Each entry
// is a literal hostname (no scheme, no port, no wildcard). Address
// literals (127.0.0.1, ::1) are accepted.
type CapNetHTTPSpec struct {
	Hosts []string `yaml:"hosts"`
}

type ManifestResources struct {
	MaxMemoryMB     uint32 `yaml:"max_memory_mb"`     // mapped to wazero MemoryLimitPages
	MaxInvocationMS uint64 `yaml:"max_invocation_ms"` // per-call deadline ceiling
	MaxFuel         uint64 `yaml:"max_fuel"`          // 0 = unbounded; reserved for Phase 2
}

// ManifestSignature carries verification metadata. Algo is fixed to
// "minisign-ed25519" for MVP; the field exists so future algorithms
// can be added without renaming.
type ManifestSignature struct {
	Algo    string `yaml:"algo"`
	KeyID   string `yaml:"key_id"`
	SigFile string `yaml:"sig_file"`
}
