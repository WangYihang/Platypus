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
	CapSysInfo CapabilityID = "sysinfo"  // host_uname: read-only host snapshot (os/arch)
	CapExec    CapabilityID = "exec"     // host_exec; requires Capabilities.Exec.Commands allowlist
	CapFSRead  CapabilityID = "fs.read"  // host_fs_read/listdir/stat; requires Capabilities.FSRead.Paths
	CapFSWrite CapabilityID = "fs.write" // host_fs_write/mkdir/chmod/rename/delete; requires Capabilities.FSWrite.Paths
	CapNetHTTP CapabilityID = "net.http" // host_http; requires Capabilities.NetHTTP.Hosts
)

// allCapabilities is the set the agent is willing to grant. Used by
// validation to reject manifests that ask for unknown capabilities.
var allCapabilities = map[CapabilityID]struct{}{
	CapLog: {}, CapKV: {}, CapSysInfo: {}, CapExec: {}, CapFSRead: {}, CapFSWrite: {}, CapNetHTTP: {},
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
	Streams      []ManifestStream     `yaml:"streams,omitempty"`
	Capabilities ManifestCapabilities `yaml:"capabilities"`
	Resources    ManifestResources    `yaml:"resources"`
	Signature    ManifestSignature    `yaml:"signature"`
}

// ManifestStream declares ownership of one wire stream type. The
// agent's stream dispatcher consults the per-Registry claim registry
// before falling into its legacy hardcoded switch — a plugin that
// claims STREAM_TYPE_PROCESS_OPEN gets that stream's bytes routed to
// it (via the host_stream_handler named in HostHandler) instead of
// the legacy agent.HandleProcessStream.
//
// Stream IO does NOT flow through wasm in MVP. The plugin is a
// claim-only entity for streams: the wasm runtime executes once at
// install time to validate the manifest and never again for stream
// dispatch. Real wasm-mediated stream IO (interleaved read/write
// inside the plugin's wasm) is the bigger-design Phase 2 work
// described in docs/plugins/STREAMING_ABI.md.
type ManifestStream struct {
	// Name is the plugin-author-facing label for the stream — useful
	// in audit logs ("plugin X handled stream foo"). Free-form;
	// uniqueness is per-plugin not global.
	Name string `yaml:"name"`

	// StreamType is the wire-level v2pb.StreamType value (string form
	// matching the proto enum: "STREAM_TYPE_PROCESS_OPEN", etc.).
	// One plugin may claim multiple stream types via multiple
	// ManifestStream entries.
	StreamType string `yaml:"stream_type"`

	// HostHandler is the registered host stream provider name to
	// delegate to. The agent's main wires the legacy handlers as
	// named providers; system plugins reference the names here. User
	// plugins (Phase 2, when wasm stream IO lands) will use a
	// reserved value like "wasm:<method_name>" to flag in-wasm
	// dispatch.
	HostHandler string `yaml:"host_handler"`
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
	FSWrite *CapFSWriteSpec `yaml:"fs.write,omitempty"`
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

// CapFSWriteSpec.Paths is the directory allowlist for fs mutations:
// create / delete / rename / chmod under one of these. Same
// component-aware + symlink-resolved enforcement as CapFSReadSpec.
type CapFSWriteSpec struct {
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
