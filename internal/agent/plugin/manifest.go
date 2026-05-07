package plugin

import "gopkg.in/yaml.v3"

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
	// CapProcess gates streaming process spawn — host_process_spawn /
	// _relay / _kill — used by the wasm replacement for the legacy
	// STREAM_TYPE_PROCESS_OPEN handler. Distinct from CapExec because
	// the blast radius is different: exec captures stdout+stderr after
	// the child completes (short-lived, bounded), whereas process spawn
	// gives the wasm an interactive PTY with stdin from the network
	// (long-lived, an operator's full shell). An operator may want to
	// grant exec without granting process; the manifest splits them so
	// the install dialog can ask separately.
	CapProcess CapabilityID = "process"
	// CapNetDial gates outbound TCP dial — host_net_dial / _relay /
	// _close — used by the wasm replacement for the legacy
	// STREAM_TYPE_TUNNEL_PULL handler. Distinct from CapNetHTTP
	// because the blast radius is fundamentally different: HTTP is a
	// scoped request/response with declared hosts, whereas net.dial
	// gives the wasm a raw bidirectional byte pipe to anywhere on the
	// agent's network — effectively SSRF capability if granted to
	// "*". The install dialog flags this prominently.
	CapNetDial CapabilityID = "net.dial"
	// CapNetListen gates inbound TCP listen — host_net_listen /
	// _accept. Distinct from CapNetDial because listening on a port
	// is a different threat model (anyone on the network can connect
	// to a bound port; operator should authorize binds explicitly).
	// Used by the new sys-tunnel-tcp / sys-tunnel-socks5 plugin
	// family for case 2 (reverse port forward) + case 3-4 (SOCKS).
	CapNetListen CapabilityID = "net.listen"
)

// AllCapabilityIDs is the canonical authority list — the order
// every UI / scaffold / docs surface uses to render or iterate
// over capability families. Code that registers per-capability
// metadata (the scaffolder's YAML / SDK-hint / description
// renderers, the FE's capability-meta table) reads this list at
// init time and asserts coverage; a future capability addition
// dropped into the constants block forces every registry to
// follow or fail loudly at startup.
var AllCapabilityIDs = []CapabilityID{
	CapLog, CapKV, CapSysInfo,
	CapExec, CapFSRead, CapFSWrite,
	CapNetHTTP, CapProcess, CapNetDial, CapNetListen,
}

// AllCapabilities returns the same list as a fresh slice so
// callers can sort / filter without mutating the package-level
// authority. Cheap (10 entries); allocates once per call.
func AllCapabilities() []CapabilityID {
	out := make([]CapabilityID, len(AllCapabilityIDs))
	copy(out, AllCapabilityIDs)
	return out
}

// allCapabilities is the lookup-friendly companion to
// AllCapabilityIDs, used by manifest validation to reject manifests
// that ask for unknown capabilities. Initialised from
// AllCapabilityIDs so adding a constant up there + the slice entry
// is the single source of truth.
var allCapabilities = func() map[CapabilityID]struct{} {
	m := make(map[CapabilityID]struct{}, len(AllCapabilityIDs))
	for _, c := range AllCapabilityIDs {
		m[c] = struct{}{}
	}
	return m
}()

// ParseCapabilityID returns the typed CapabilityID for a wire-name
// string (e.g. the "fs.read" the scaffolder receives via
// --capabilities) or false when the string isn't one of the
// declared families. Centralising parsing means a typo at the CLI
// boundary fails before any file is written, with a clear list of
// the valid options.
func ParseCapabilityID(s string) (CapabilityID, bool) {
	c := CapabilityID(s)
	_, ok := allCapabilities[c]
	if !ok {
		return "", false
	}
	return c, true
}

// CapabilityIDsFromStrings is the proto-side adapter: protobuf
// generates `[]string` for repeated capability fields, and we
// promote at the boundary to the typed slice every internal Go
// caller works with. Unknown families are silently dropped — the
// agent's separate ValidateGranted call surfaces the rejection
// with a precise reason; this function is for "convert what came
// off the wire".
func CapabilityIDsFromStrings(in []string) []CapabilityID {
	if len(in) == 0 {
		return nil
	}
	out := make([]CapabilityID, 0, len(in))
	for _, s := range in {
		if c, ok := ParseCapabilityID(s); ok {
			out = append(out, c)
		}
	}
	return out
}

// CapabilityIDsToStrings is the inverse adapter, used when
// building a wire-shape `repeated string` for proto. The naming
// mirrors CapabilityIDsFromStrings so call sites read symmetrically.
func CapabilityIDsToStrings(in []CapabilityID) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, string(c))
	}
	return out
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
	// Config declares the deployment-time configuration shape for
	// this plugin. Optional — plugins that take all their parameters
	// at RPC-call time can leave it empty and the install path
	// behaves as it always has. When present, the operator-facing UI
	// renders a form against the schema, the server validates
	// supplied configs against it, and the agent SDK delivers a
	// validated config blob to the plugin's OnInstall hook.
	Config ManifestConfig `yaml:"config,omitempty"`
}

// ManifestConfig is the authoring-side declaration of a plugin's
// deployment-time configuration. The shape mirrors what the
// frontend's schema-driven editor consumes:
//
//   - Schema: a JSON Schema (draft 2020-12 by convention) describing
//     valid configurations. We pass the schema through verbatim — Go
//     never materialises it as an AST. yaml.Node is the
//     pass-through-friendly representation; validation happens via
//     gojsonschema (or similar) in manifest_validate.go.
//   - Defaults: optional initial values that get merged with the
//     schema's `default:` keywords at resolve time. Saved configs
//     store only the operator's overrides; this lets plugin authors
//     change defaults without rewriting every deployed config.
//   - SecretFields: JSON Pointer-style paths into the schema marking
//     fields as sensitive. The UI renders password inputs / "use
//     saved secret" pickers for these; storage swaps the inline
//     value for a {"$secret": "sec_..."} reference; logs and
//     responses redact them.
//   - SchemaVersion: bumped on breaking changes. PluginSpec carries
//     the schema_version it was authored against; the resolver
//     refuses to deploy a config whose schema_version doesn't match
//     the manifest currently published, surfacing a clear
//     "operator must reauthor" prompt rather than silent breakage.
type ManifestConfig struct {
	Schema        yaml.Node `yaml:"schema,omitempty"`
	Defaults      yaml.Node `yaml:"defaults,omitempty"`
	SecretFields  []string  `yaml:"secret_fields,omitempty"`
	SchemaVersion int       `yaml:"schema_version,omitempty"`
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
	// Lang is informational (the agent doesn't branch on it). Useful
	// for the web UI / authoring docs to render a "written in Rust"
	// vs "written in Go" badge. Plugin authors are expected to set
	// this honestly so the operator-facing tooling can surface it;
	// it's not a security boundary.
	Lang string `yaml:"lang,omitempty"`

	// OSTargets / ArchTargets restrict which agents the server's
	// reconciler will push this plugin to. Values are Go's
	// runtime.GOOS / runtime.GOARCH strings ("linux", "darwin",
	// "windows" / "amd64", "arm64", ...). Empty slice = match all,
	// which preserves backward compatibility with manifests written
	// before these fields existed.
	//
	// The check is enforced server-side in reconcileSystemPlugins;
	// the agent itself doesn't validate OSTargets at install time
	// (an operator who manually pushes a linux-only plugin to a
	// darwin agent will simply see the wasm fail to do anything
	// useful — the host_fs_read of /proc returns "not found").
	OSTargets   []string `yaml:"os_targets,omitempty"`
	ArchTargets []string `yaml:"arch_targets,omitempty"`
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
	Exec      *CapExecSpec      `yaml:"exec,omitempty"`
	FSRead    *CapFSReadSpec    `yaml:"fs.read,omitempty"`
	FSWrite   *CapFSWriteSpec   `yaml:"fs.write,omitempty"`
	NetHTTP   *CapNetHTTPSpec   `yaml:"net.http,omitempty"`
	Process   *CapProcessSpec   `yaml:"process,omitempty"`
	NetDial   *CapNetDialSpec   `yaml:"net.dial,omitempty"`
	NetListen *CapNetListenSpec `yaml:"net.listen,omitempty"`
	KV        bool              `yaml:"kv,omitempty"`
	SysInfo   bool              `yaml:"sysinfo,omitempty"`
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

// CapProcessSpec.Commands is the exact-path allowlist for streaming
// process spawn. Same posture as CapExecSpec: literal paths only, no
// wildcards, so an operator reading the manifest knows precisely
// which binaries the plugin can launch as an interactive PTY.
type CapProcessSpec struct {
	Commands []string `yaml:"commands"`
}

// CapNetDialSpec.Targets is the exact "host:port" allowlist for
// outbound TCP dial. Each entry is a literal target ("10.0.0.5:22",
// "internal-svc:8080") or "*" for unrestricted. The legacy
// STREAM_TYPE_TUNNEL_PULL handler had implicit any-target authority,
// so the system bundle's sys-tunnel-tcp uses "*"; a third-party
// replacement should narrow to a literal list and the install
// dialog flags "*" prominently.
type CapNetDialSpec struct {
	Targets []string `yaml:"targets"`
}

// CapNetListenSpec.Binds is the allowlist of bind addresses
// host_net_listen will accept. Each entry is a literal "host:port"
// or a glob (`127.0.0.1:1080`, `0.0.0.0:8080`, `*:1080`,
// `127.0.0.1:1024-65535` is NOT supported — use repeat entries or
// a glob like `127.0.0.1:*` and trust the operator's review).
//
// The implicit unrestricted entry is "*:*" — operators should think
// twice before granting it because a plugin with this allowance can
// bind privileged ports (1-1023) if the agent runs as root and use
// them as covert exfil endpoints.
type CapNetListenSpec struct {
	Binds []string `yaml:"binds"`
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
