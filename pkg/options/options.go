// Package options is the agent's command-line surface. The shape is
// intentionally minimal: most users see one positional argument
// (the install token) and never touch a flag.
//
// Resolution order for every value below is identical:
//
//	explicit flag → env var → persisted state on disk → default
//
// so an admin who pre-stages PSK + identity-dir via Ansible doesn't
// have to thread anything through the binary's invocation, and a
// one-shot manual run can override any of it from the shell.
package options

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/pkg/version"
)

// Subcommand classifies the CLI invocation. The agent has exactly two
// modes: run-as-an-agent (default) and admin/setup helpers (psk).
// Anything else is a parse error.
type Subcommand int

const (
	// SubcommandRun is the default — `platypus-agent <install-token>`
	// (or env-var equivalents) and we go straight into the
	// enroll → link loop.
	SubcommandRun Subcommand = iota
	// SubcommandPSKInstall writes the supplied PSK to disk under the
	// resolved data-dir and exits 0. Admins run this once during
	// fleet provisioning so subsequent agent invocations pick up
	// the PSK automatically.
	SubcommandPSKInstall
	// SubcommandPSKShow prints the resolved PSK file path (and
	// whether it exists) for debugging. Doesn't print PSK contents.
	SubcommandPSKShow
)

// Options is the parsed CLI surface used by the agent main loop.
// Most production runs only populate Token + DataDir + the resolved
// PSK fields; the rest stay at zero.
type Options struct {
	Sub Subcommand

	// Run-mode fields ---------------------------------------------------

	// Token is the install token (PAT) the agent redeems for a cert
	// + CA on first run. Subsequent runs find a stored identity and
	// skip enrollment. Required for SubcommandRun on a fresh
	// install; ignored once an identity is on disk (the
	// PLATYPUS_PROJECT_CA env var is the active-pointer marker
	// instead).
	Token string

	// RemoteHost / RemotePort identify the server the agent dials.
	// Filled either from --host/--port or extracted from the
	// install token's leading "host:port@" prefix when present.
	// Empty when relying on persisted state from a prior bootstrap.
	RemoteHost string
	RemotePort int

	// DataDir is the writable root the agent persists identity +
	// per-CA layout under. Defaults to ~/.platypus/agent (or
	// /var/lib/platypus when running as root).
	DataDir string

	// Mesh subset --------------------------------------------------------

	// MeshPSKFile is the absolute path to the mesh pre-shared key.
	// Resolution order is documented at PSKResolutionOrder below.
	// Empty after resolution means "no PSK installed" — mesh
	// participation is disabled until one is provided.
	MeshPSKFile string

	// MeshListen / MeshAdvertise / MeshDiscoveryLAN are kept as
	// hidden flags for power users; defaults work for >99% of
	// deployments. The DiscoveryLAN default is true so a fresh
	// install with `psk install` set up auto-joins peers on the
	// LAN without further configuration.
	MeshListen            string
	MeshPeers             []string
	MeshAdvertise         []string
	MeshDiscoveryLAN      bool
	MeshDiscoveryInterval int

	// PSK-mode fields ---------------------------------------------------
	PSKArg string // the PSK string (base32 or hex) for `psk install`
}

// Env-var names exposed for tests / docs / install scripts so a
// rename here propagates everywhere mechanically.
const (
	EnvInstallToken = "PLATYPUS_INSTALL_TOKEN"
	EnvServerAddr   = "PLATYPUS_SERVER" // "host:port"
	EnvDataDir      = "PLATYPUS_DATA_DIR"
	EnvMeshPSK      = "PLATYPUS_MESH_PSK"      // raw base32, in-memory
	EnvMeshPSKFile  = "PLATYPUS_MESH_PSK_FILE" // path on disk
)

// ErrMissingToken is returned when SubcommandRun was inferred but no
// install token surfaced anywhere in the resolution chain. Main turns
// this into a friendly usage message.
var ErrMissingToken = errors.New("install token required")

// ErrMissingServer is returned when SubcommandRun was inferred but
// neither --host/--port nor PLATYPUS_SERVER produced a server address
// AND no persisted identity exists under DataDir. The agent has
// nowhere to dial.
var ErrMissingServer = errors.New("server address required")

// PSKResolutionOrder documents how the agent picks up the mesh PSK
// at runtime. Listed in resolution order. Higher entries override
// lower ones.
//
//  1. --psk-file <path>  (explicit flag, escape hatch)
//  2. PLATYPUS_MESH_PSK_FILE  (path, env var)
//  3. PLATYPUS_MESH_PSK       (inline base32, env var — file written ephemerally)
//  4. <data-dir>/mesh.psk     (default location, written by `psk install`)
//  5. /etc/platypus/mesh.psk  (system-wide fallback for service deployments)
const PSKResolutionOrder = `--psk-file > $PLATYPUS_MESH_PSK_FILE > $PLATYPUS_MESH_PSK > <data-dir>/mesh.psk > /etc/platypus/mesh.psk`

// InitOptions parses os.Args using the new minimal surface. Returns
// either a populated Options + nil err on success, or a partially
// populated Options + a sentinel error when a required value couldn't
// be resolved (so callers can render a contextual help message).
func InitOptions() (*Options, error) {
	return parseArgs(os.Args[1:], os.Getenv)
}

// parseArgs is InitOptions with explicit injection points so tests
// can drive the parser without touching the global env / argv.
func parseArgs(argv []string, env func(string) string) (*Options, error) {
	opts := &Options{
		MeshDiscoveryLAN:      true,
		MeshDiscoveryInterval: 30,
	}

	// Subcommand dispatch up-front so flag.Parse only sees the
	// flags relevant to the inferred mode. `psk` is the only
	// non-default verb; everything else is positional / flags for
	// the run-mode.
	if len(argv) > 0 && argv[0] == "psk" {
		return parsePSK(argv[1:], opts)
	}

	fs := flag.NewFlagSet("platypus-agent", flag.ContinueOnError)
	fs.SetOutput(newDeferredWriter())
	var (
		showVersion bool
		host        string
		port        int
	)
	// Hidden, low-traffic knobs. Documented in the help block at
	// the top of the file but kept off the standard usage line so
	// the canonical invocation stays a single positional argument.
	fs.StringVar(&opts.DataDir, "data-dir", "", "writable directory for persistent state (default: ~/.platypus/agent)")
	fs.StringVar(&opts.MeshPSKFile, "psk-file", "", "explicit path to mesh PSK (default: auto-resolved, see -help)")
	fs.StringVar(&host, "host", "", "server hostname/IP (overrides token-embedded value)")
	fs.IntVar(&port, "port", 0, "server port (overrides token-embedded value)")
	fs.StringVar(&opts.MeshListen, "mesh-listen", "", "address to accept inbound mesh links (e.g. :17777). Empty = leaf-only.")
	fs.BoolVar(&opts.MeshDiscoveryLAN, "mesh-discovery", true, "enable mDNS LAN discovery")
	fs.IntVar(&opts.MeshDiscoveryInterval, "mesh-discovery-interval", 30, "mDNS scan interval (seconds)")
	stringSliceVar(fs, &opts.MeshAdvertise, "mesh-advertise", "override advertised mesh listen address(es) (repeatable)")
	stringSliceVar(fs, &opts.MeshPeers, "peers", "explicit mesh bootstrap peer (host:port, repeatable)")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")

	if err := fs.Parse(argv); err != nil {
		return nil, err
	}
	if showVersion {
		version.PrintVersion()
		os.Exit(0)
	}

	opts.RemoteHost = host
	opts.RemotePort = port

	// Positional arg: the install token. We only consume the first;
	// extras are parse errors so a fat-fingered flag doesn't
	// silently slide past as a positional.
	rest := fs.Args()
	switch len(rest) {
	case 0:
		// fall through — env var may still supply the token
	case 1:
		opts.Token = rest[0]
	default:
		return nil, fmt.Errorf("unexpected extra arguments after install token: %v", rest[1:])
	}

	// Env-var fallbacks for everything that has one.
	if opts.Token == "" {
		opts.Token = env(EnvInstallToken)
	}
	if opts.DataDir == "" {
		opts.DataDir = env(EnvDataDir)
	}
	if opts.RemoteHost == "" {
		if srv := env(EnvServerAddr); srv != "" {
			h, p, err := splitHostPort(srv)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", EnvServerAddr, err)
			}
			opts.RemoteHost, opts.RemotePort = h, p
		}
	}

	// Token may also encode the server endpoint as a "host:port@token"
	// prefix. Lets a copy-pasted token Just Work without --host/--port.
	if h, p, t, ok := splitTokenWithServer(opts.Token); ok {
		opts.Token = t
		if opts.RemoteHost == "" {
			opts.RemoteHost = h
			opts.RemotePort = p
		}
	}

	return opts, nil
}

// parsePSK handles `platypus-agent psk <verb> [args]`. Two verbs:
//
//	install <psk>   — write to <data-dir>/mesh.psk (0600)
//	show            — print resolved PSK file path
func parsePSK(argv []string, opts *Options) (*Options, error) {
	if len(argv) == 0 {
		return nil, errors.New("psk: missing verb (install|show)")
	}
	verb := argv[0]
	fs := flag.NewFlagSet("platypus-agent psk", flag.ContinueOnError)
	fs.SetOutput(newDeferredWriter())
	fs.StringVar(&opts.DataDir, "data-dir", "", "writable directory for persistent state (default: ~/.platypus/agent)")
	fs.StringVar(&opts.MeshPSKFile, "psk-file", "", "override target path (default: <data-dir>/mesh.psk)")
	if err := fs.Parse(argv[1:]); err != nil {
		return nil, err
	}
	switch verb {
	case "install":
		opts.Sub = SubcommandPSKInstall
		rest := fs.Args()
		if len(rest) == 0 {
			return nil, errors.New("psk install: PSK argument required")
		}
		if len(rest) > 1 {
			return nil, fmt.Errorf("psk install: unexpected extra args %v", rest[1:])
		}
		opts.PSKArg = rest[0]
		return opts, nil
	case "show":
		opts.Sub = SubcommandPSKShow
		return opts, nil
	default:
		return nil, fmt.Errorf("psk: unknown verb %q (want install|show)", verb)
	}
}

// splitTokenWithServer parses a "host:port@<token>" prefix off the
// token string. Returns ok=false when no @ is present, leaving the
// token unchanged. Lets the install dialog ship a single
// copy-pasteable string without an explicit --host flag.
func splitTokenWithServer(raw string) (host string, port int, token string, ok bool) {
	at := strings.Index(raw, "@")
	if at <= 0 {
		return "", 0, raw, false
	}
	hp := raw[:at]
	rest := raw[at+1:]
	h, p, err := splitHostPort(hp)
	if err != nil {
		return "", 0, raw, false
	}
	return h, p, rest, true
}

func splitHostPort(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("expected host:port, got %q", s)
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, fmt.Errorf("port: %w", err)
	}
	return s[:i], port, nil
}

// stringSliceVar registers a repeatable flag (`--name X --name Y`).
// flag.StringSliceVar is in pflag, not stdlib, so we roll our own
// rather than pull a dependency just for one knob.
func stringSliceVar(fs *flag.FlagSet, dst *[]string, name, usage string) {
	fs.Func(name, usage, func(s string) error {
		*dst = append(*dst, s)
		return nil
	})
}

// deferredWriter discards parser output so we can render a
// hand-written usage block instead of stdlib's default. flag.Parse
// otherwise prints "flag provided but not defined: -foo" + its full
// flag dump on stderr, drowning the friendly message.
type deferredWriter struct{}

func newDeferredWriter() *deferredWriter { return &deferredWriter{} }
func (deferredWriter) Write(p []byte) (int, error) {
	// Echo to stderr without the flag dump prefix; the parser still
	// produces non-zero exit via the returned error.
	if len(p) == 0 {
		return 0, nil
	}
	_, _ = os.Stderr.Write(p)
	return len(p), nil
}
