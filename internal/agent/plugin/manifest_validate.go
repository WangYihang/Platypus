package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// idRegexp matches reverse-DNS plugin ids: lowercase, dot-separated,
// each segment alphanumeric or hyphen. Strict on purpose so an id can
// be used safely as a filesystem path component.
var idRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

// versionRegexp matches strict semver MAJOR.MINOR.PATCH (no pre-release
// or build metadata). MVP keeps it tight; extensions can come later.
var versionRegexp = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ValidatePluginID is the public wrapper around idRegexp the
// scaffolder CLI's huh.Validate callback calls. Returns nil for a
// valid reverse-DNS id (lowercase, dot-separated segments, each
// alphanumeric-or-hyphen) and a descriptive error otherwise.
// Sharing one regex with the manifest validator means the wizard
// never produces an id the server-side validator would reject.
func ValidatePluginID(id string) error {
	if id == "" {
		return errors.New("plugin id is required")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("plugin id %q must be reverse-DNS (lowercase, dot-separated, e.g. com.example.my-plugin)", id)
	}
	return nil
}

// ValidateSemver is the public wrapper around versionRegexp. Same
// posture as ValidatePluginID: identical regex to the manifest
// validator so the scaffolder's wizard rejects bad versions before
// the author hits save.
func ValidateSemver(v string) error {
	if v == "" {
		return errors.New("version is required")
	}
	if !versionRegexp.MatchString(v) {
		return fmt.Errorf("version %q must be strict semver MAJOR.MINOR.PATCH (no pre-release / build metadata)", v)
	}
	return nil
}

// ParseManifest unmarshals + validates a plugin.yaml. It does not touch
// the filesystem; pass the raw bytes already loaded by the caller.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin: parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate enforces every constraint we want at parse time. Errors are
// joined so an author sees them all at once rather than one fix-then-
// see-the-next per round trip.
func (m *Manifest) Validate() error {
	var errs []error
	if m.APIVersion != 1 {
		errs = append(errs, fmt.Errorf("api_version=%d, only 1 is supported", m.APIVersion))
	}
	if !idRegexp.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("id=%q is not a valid reverse-DNS identifier", m.ID))
	}
	if m.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if !versionRegexp.MatchString(m.Version) {
		errs = append(errs, fmt.Errorf("version=%q is not strict semver MAJOR.MINOR.PATCH", m.Version))
	}
	if m.Runtime.Type != "wasm" {
		errs = append(errs, fmt.Errorf("runtime.type=%q, only \"wasm\" is supported", m.Runtime.Type))
	}
	if m.Runtime.Entry == "" {
		errs = append(errs, errors.New("runtime.entry is required"))
	} else if filepath.Base(m.Runtime.Entry) != m.Runtime.Entry {
		// Reject ../foo.wasm or absolute paths — entry MUST be a plain
		// filename to be joined safely under VersionDir.
		errs = append(errs, fmt.Errorf("runtime.entry=%q must be a plain filename, no path components",
			m.Runtime.Entry))
	}
	if m.Runtime.ABI != "extism/1" {
		errs = append(errs, fmt.Errorf("runtime.abi=%q, only \"extism/1\" is supported", m.Runtime.ABI))
	}
	if len(m.RPC) == 0 && len(m.Streams) == 0 {
		errs = append(errs, errors.New("at least one rpc or streams entry is required"))
	}
	seen := map[string]bool{}
	for i, r := range m.RPC {
		if r.Name == "" {
			errs = append(errs, fmt.Errorf("rpc[%d].name is required", i))
			continue
		}
		if seen[r.Name] {
			errs = append(errs, fmt.Errorf("rpc[%d].name=%q is duplicated", i, r.Name))
		}
		seen[r.Name] = true
	}
	streamSeen := map[string]bool{}
	for i, s := range m.Streams {
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("streams[%d].name is required", i))
			continue
		}
		if streamSeen[s.Name] {
			errs = append(errs, fmt.Errorf("streams[%d].name=%q is duplicated", i, s.Name))
		}
		streamSeen[s.Name] = true
		if s.StreamType == "" {
			errs = append(errs, fmt.Errorf("streams[%d].stream_type is required", i))
		}
		if s.HostHandler == "" {
			errs = append(errs, fmt.Errorf("streams[%d].host_handler is required", i))
		}
	}
	if err := m.Capabilities.validate(); err != nil {
		errs = append(errs, err)
	}
	if m.Resources.MaxMemoryMB == 0 {
		errs = append(errs, errors.New("resources.max_memory_mb is required (must be > 0)"))
	} else if m.Resources.MaxMemoryMB > 1024 {
		errs = append(errs, fmt.Errorf("resources.max_memory_mb=%d exceeds the 1024 MB ceiling",
			m.Resources.MaxMemoryMB))
	}
	if m.Resources.MaxInvocationMS == 0 {
		errs = append(errs, errors.New("resources.max_invocation_ms is required (must be > 0)"))
	}
	if m.Signature.Algo != "minisign-ed25519" {
		errs = append(errs, fmt.Errorf("signature.algo=%q, only \"minisign-ed25519\" is supported",
			m.Signature.Algo))
	}
	if m.Signature.KeyID == "" {
		errs = append(errs, errors.New("signature.key_id is required"))
	}
	if m.Signature.SigFile == "" {
		errs = append(errs, errors.New("signature.sig_file is required"))
	} else if filepath.Base(m.Signature.SigFile) != m.Signature.SigFile {
		errs = append(errs, fmt.Errorf("signature.sig_file=%q must be a plain filename",
			m.Signature.SigFile))
	}
	return errors.Join(errs...)
}

// isAbsCrossPlatform accepts paths absolute under either POSIX OR
// Windows conventions, regardless of the host OS doing validation.
// stdlib's filepath.IsAbs is host-native (linux server can't validate
// "C:\\..." manifests); we need explicit cross-platform recognition
// since the same staged manifest tree is consumed by linux + darwin
// + windows agents from a single server build.
//
// Accepted forms:
//   - "/abs/posix/path"
//   - "C:\\Windows\\..." or "C:/Windows/..." (any drive letter)
//   - "\\\\server\\share\\..." (UNC)
func isAbsCrossPlatform(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	// Windows drive letter (X:\ or X:/).
	if len(p) >= 3 {
		c := p[0]
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if isLetter && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
			return true
		}
	}
	// UNC.
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return false
}

func (c ManifestCapabilities) validate() error {
	var errs []error
	if c.Exec != nil {
		if len(c.Exec.Commands) == 0 {
			errs = append(errs, errors.New("capabilities.exec set without any commands"))
		}
		for i, cmd := range c.Exec.Commands {
			if cmd == "*" {
				// Unrestricted-exec marker. Validation accepts this only
				// because it's necessary for system-plugin migrations of
				// the legacy any-command Exec handler; the operator-side
				// capability dialog is responsible for surfacing the
				// "*" entry prominently for third-party plugins.
				continue
			}
			if !isAbsCrossPlatform(cmd) {
				errs = append(errs, fmt.Errorf(
					"capabilities.exec.commands[%d]=%q must be an absolute path or \"*\"", i, cmd))
			}
		}
	}
	if c.FSRead != nil {
		if len(c.FSRead.Paths) == 0 {
			errs = append(errs, errors.New("capabilities.fs.read set without any paths"))
		}
		for i, p := range c.FSRead.Paths {
			if !isAbsCrossPlatform(p) {
				errs = append(errs, fmt.Errorf(
					"capabilities.fs.read.paths[%d]=%q must be an absolute path", i, p))
			}
		}
	}
	if c.FSWrite != nil {
		if len(c.FSWrite.Paths) == 0 {
			errs = append(errs, errors.New("capabilities.fs.write set without any paths"))
		}
		for i, p := range c.FSWrite.Paths {
			if !isAbsCrossPlatform(p) {
				errs = append(errs, fmt.Errorf(
					"capabilities.fs.write.paths[%d]=%q must be an absolute path", i, p))
			}
		}
	}
	if c.NetHTTP != nil {
		if len(c.NetHTTP.Hosts) == 0 {
			errs = append(errs, errors.New("capabilities.net.http set without any hosts"))
		}
		for i, h := range c.NetHTTP.Hosts {
			if h == "" || strings.ContainsAny(h, "/?#: ") {
				errs = append(errs, fmt.Errorf(
					"capabilities.net.http.hosts[%d]=%q must be a bare hostname (no scheme/port/path)", i, h))
			}
		}
	}
	if c.Process != nil {
		if len(c.Process.Commands) == 0 {
			errs = append(errs, errors.New("capabilities.process set without any commands"))
		}
		for i, cmd := range c.Process.Commands {
			if cmd == "*" {
				// Unrestricted-spawn marker; same posture as exec.
				// Required for the wasm replacement of the legacy
				// PROCESS_OPEN handler which had implicit any-command
				// access via the agent's process group.
				continue
			}
			if !isAbsCrossPlatform(cmd) {
				errs = append(errs, fmt.Errorf(
					"capabilities.process.commands[%d]=%q must be an absolute path or \"*\"", i, cmd))
			}
		}
	}
	if c.NetDial != nil {
		if len(c.NetDial.Targets) == 0 {
			errs = append(errs, errors.New("capabilities.net.dial set without any targets"))
		}
		for i, t := range c.NetDial.Targets {
			if t == "*" {
				// Unrestricted-dial marker. Effectively SSRF
				// authority — only sane for the system bundle. The
				// install-time capability dialog flags this.
				continue
			}
			if t == "" || strings.ContainsAny(t, " \t/") {
				errs = append(errs, fmt.Errorf(
					"capabilities.net.dial.targets[%d]=%q must be host:port or \"*\"", i, t))
				continue
			}
			if !strings.Contains(t, ":") {
				errs = append(errs, fmt.Errorf(
					"capabilities.net.dial.targets[%d]=%q missing :port", i, t))
			}
		}
	}
	if c.NetListen != nil {
		if len(c.NetListen.Binds) == 0 {
			errs = append(errs, errors.New("capabilities.net.listen set without any binds"))
		}
		for i, b := range c.NetListen.Binds {
			// "*:*" — fully unrestricted bind authority. Same posture
			// as net.dial "*": only sane for system-bundle plugins
			// the operator deliberately authorizes; the install dialog
			// flags it.
			if b == "*:*" {
				continue
			}
			if b == "" || strings.ContainsAny(b, " \t/") {
				errs = append(errs, fmt.Errorf(
					"capabilities.net.listen.binds[%d]=%q must be host:port or \"*:*\"", i, b))
				continue
			}
			if !strings.Contains(b, ":") {
				errs = append(errs, fmt.Errorf(
					"capabilities.net.listen.binds[%d]=%q missing :port", i, b))
			}
		}
	}
	return errors.Join(errs...)
}

// DeclaredCapabilities returns the set of capability ids the manifest
// requests. Used at install time to render the operator-confirmation
// dialog and to validate that every PluginInstallRequest.granted_caps
// entry is actually requested by the manifest.
func (m *Manifest) DeclaredCapabilities() []CapabilityID {
	var out []CapabilityID
	out = append(out, CapLog) // always granted; here for completeness in audit views
	if m.Capabilities.KV {
		out = append(out, CapKV)
	}
	if m.Capabilities.SysInfo {
		out = append(out, CapSysInfo)
	}
	if m.Capabilities.Exec != nil {
		out = append(out, CapExec)
	}
	if m.Capabilities.FSRead != nil {
		out = append(out, CapFSRead)
	}
	if m.Capabilities.FSWrite != nil {
		out = append(out, CapFSWrite)
	}
	if m.Capabilities.NetHTTP != nil {
		out = append(out, CapNetHTTP)
	}
	if m.Capabilities.Process != nil {
		out = append(out, CapProcess)
	}
	if m.Capabilities.NetDial != nil {
		out = append(out, CapNetDial)
	}
	if m.Capabilities.NetListen != nil {
		out = append(out, CapNetListen)
	}
	return out
}

// ValidateGranted ensures the granted set is a subset of the declared
// set. Returns the unknown / overgranted entries (both as a single
// error) so the caller can surface a precise install-rejection reason.
func (m *Manifest) ValidateGranted(granted []CapabilityID) error {
	declared := map[CapabilityID]bool{}
	for _, c := range m.DeclaredCapabilities() {
		declared[c] = true
	}
	var bad []string
	for _, g := range granted {
		if _, ok := allCapabilities[g]; !ok {
			bad = append(bad, string(g)+" (unknown)")
			continue
		}
		if !declared[g] {
			bad = append(bad, string(g)+" (not requested by manifest)")
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("plugin: capability_overgrant: %s", strings.Join(bad, ", "))
	}
	return nil
}
