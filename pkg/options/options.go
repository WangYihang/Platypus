// Package options is the platypus-agent command-line surface.
// Parsing runs through alecthomas/kong, which gives every value an
// identical resolution chain:
//
//	explicit flag → env var → default value (struct-tag)
//
// On top of that the agent overlays a third source — persisted state
// on disk — for fields where a re-running agent should pick up what
// the previous bootstrap stamped (PSK file, server endpoint embedded
// in the cert, etc.). That layer lives in main.go, not here.
//
// The agent has three CLI verbs:
//
//	platypus-agent <install-token>          run-mode (default)
//	platypus-agent psk install <psk>        write PSK to <data-dir>/mesh.psk
//	platypus-agent psk show                 print resolved PSK file path
//
// Each verb is a separate kong sub-command. Their flag sets overlap
// (--data-dir / --psk-file) so admins typing `psk` after a long run
// command don't have to remember which set is which. Whatever the
// matched command, InitOptions returns one fully-populated Options;
// callers branch on Sub.
package options

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/WangYihang/Platypus/pkg/version"
)

// Subcommand classifies the parsed CLI invocation. The agent has
// exactly three modes; SubcommandRun is the default.
type Subcommand int

const (
	SubcommandRun Subcommand = iota
	SubcommandPSKInstall
	SubcommandPSKShow
)

// Options is the parsed CLI surface used by main.go. It's
// intentionally a flat struct (not the nested kong CLI tree) so main
// only needs to switch on Sub and read the fields that apply to that
// mode. The InitOptions function flattens kong's parse result into
// this shape.
type Options struct {
	Sub Subcommand

	// Run-mode fields ---------------------------------------------------

	Token      string
	RemoteHost string
	RemotePort int

	// Common to all subcommands.
	DataDir     string
	MeshPSKFile string

	// Run-mode mesh knobs.
	MeshListen            string
	MeshPeers             []string
	MeshAdvertise         []string
	MeshDiscoveryLAN      bool
	MeshDiscoveryInterval int

	// PSK-mode fields ---------------------------------------------------
	PSKArg string
}

// Env-var names exposed for tests / docs / install scripts so a
// rename here propagates everywhere mechanically.
const (
	EnvInstallToken = "PLATYPUS_INSTALL_TOKEN"
	EnvServerAddr   = "PLATYPUS_SERVER"
	EnvDataDir      = "PLATYPUS_DATA_DIR"
	EnvMeshPSK      = "PLATYPUS_MESH_PSK"
	EnvMeshPSKFile  = "PLATYPUS_MESH_PSK_FILE"
)

// ErrMissingToken / ErrMissingServer are surfaced by main when a fresh
// install is missing the bootstrap inputs. Returned through Options
// so main can render a contextual usage message rather than letting
// kong print its own (which is built for a missing flag, not a
// missing combination).
var (
	ErrMissingToken  = errors.New("install token required")
	ErrMissingServer = errors.New("server address required")
)

// PSKResolutionOrder documents how the agent picks up the mesh PSK
// at runtime. See Options.MeshPSKFile + EnvMeshPSK / EnvMeshPSKFile +
// DataDir for the actual resolution.
const PSKResolutionOrder = `--psk-file > $PLATYPUS_MESH_PSK_FILE > $PLATYPUS_MESH_PSK > <data-dir>/mesh.psk > /etc/platypus/mesh.psk`

// --- kong tree ----------------------------------------------------
//
// runCmd is the default verb; kong matches it when the first arg
// isn't `psk`. The install-token is positional + optional so reruns
// without a token (with persisted identity already on disk) work.

type runCmd struct {
	Token string `arg:"" optional:"" name:"install-token" env:"PLATYPUS_INSTALL_TOKEN" help:"Install token (PAT) to redeem on first run; ignored after enrollment. May embed server as 'host:port@<token>', or be a 'pinst_' install bundle."`

	Server string `name:"server" env:"PLATYPUS_SERVER" placeholder:"HOST:PORT" help:"Server endpoint. Falls back to the embedded prefix in --install-token (when present) or persisted state."`

	DataDir     string `name:"data-dir" env:"PLATYPUS_DATA_DIR" placeholder:"PATH" help:"Persistent state root. Defaults to ~/.platypus/agent (or /var/lib/platypus when running as root)."`
	MeshPSKFile string `name:"psk-file" placeholder:"PATH" help:"Override mesh PSK path (default: <data-dir>/mesh.psk)."`

	MeshListen            string   `name:"mesh-listen" placeholder:"HOST:PORT" help:"Address to accept inbound mesh links. Empty = leaf-only (the typical configuration)."`
	MeshDiscoveryLAN      bool     `name:"mesh-discovery" default:"true" negatable:"" help:"Enable mDNS LAN discovery."`
	MeshDiscoveryInterval int      `name:"mesh-discovery-interval" default:"30" placeholder:"SECS" help:"mDNS scan interval (seconds)."`
	MeshAdvertise         []string `name:"mesh-advertise" placeholder:"HOST:PORT" help:"Override advertised mesh listen address(es). Repeatable."`
	MeshPeers             []string `name:"peers" placeholder:"HOST:PORT" help:"Explicit mesh bootstrap peer. Repeatable."`
}

// pskCmd is the `psk` verb with two sub-verbs. Both share the same
// data-dir / psk-file knobs because operators often have just one
// data-dir and run install + show against it back-to-back.

type pskCmd struct {
	Install pskInstallCmd `cmd:"" help:"Write a PSK to <data-dir>/mesh.psk (mode 0600) and exit."`
	Show    pskShowCmd    `cmd:"" help:"Print the resolved PSK file path and whether it exists."`
}

type pskInstallCmd struct {
	PSK         string `arg:"" placeholder:"PSK" help:"PSK string (base32 or hex)."`
	DataDir     string `name:"data-dir" env:"PLATYPUS_DATA_DIR" placeholder:"PATH" help:"Persistent state root."`
	MeshPSKFile string `name:"psk-file" placeholder:"PATH" help:"Override target path (default: <data-dir>/mesh.psk)."`
}

type pskShowCmd struct {
	DataDir     string `name:"data-dir" env:"PLATYPUS_DATA_DIR" placeholder:"PATH" help:"Persistent state root."`
	MeshPSKFile string `name:"psk-file" placeholder:"PATH" help:"Override target path."`
}

// cli is the root struct kong unmarshals into.
type cli struct {
	Run runCmd `cmd:"" default:"withargs" help:"Run the agent (default verb when no sub-command is given)."`
	PSK pskCmd `cmd:"psk" help:"Manage the mesh pre-shared key."`

	Version kong.VersionFlag `short:"v" help:"Print version and exit."`
}

// InitOptions parses os.Args and returns a populated Options. Calls
// kong.Parse, which prints a kong-formatted usage on failure and
// exits — so the only error path that returns through here is the
// one we want main to handle gracefully (missing token / server when
// no persisted identity exists; that check stays in main, not here).
func InitOptions() (*Options, error) {
	return parseArgs(os.Args[1:])
}

// parseArgs is InitOptions with explicit injection points so tests
// can drive the parser without touching the global env / argv. Tests
// pass their own t.Setenv via testing.T.
func parseArgs(argv []string) (*Options, error) {
	var c cli
	parser, err := kong.New(&c,
		kong.Name("platypus-agent"),
		kong.Description("Platypus managed-host agent. Run with an install token on first boot; subsequent runs reuse the persisted identity."),
		kong.Vars{"version": version.Version},
		kong.UsageOnError(),
	)
	if err != nil {
		return nil, fmt.Errorf("agent options: kong.New: %w", err)
	}
	kctx, err := parser.Parse(argv)
	if err != nil {
		return nil, err
	}
	return flatten(&c, kctx.Command())
}

// flatten projects whichever kong sub-command matched into the flat
// Options struct main.go expects. The choice of "flat-out-of-tree"
// rather than "main.go reads kong tree directly" is deliberate:
// main.go was designed against a flat struct and rewriting that
// just to track kong's command tree would churn 30+ call sites
// without making any of them clearer.
func flatten(c *cli, command string) (*Options, error) {
	switch command {
	case "psk install <psk>":
		return &Options{
			Sub:         SubcommandPSKInstall,
			DataDir:     c.PSK.Install.DataDir,
			MeshPSKFile: c.PSK.Install.MeshPSKFile,
			PSKArg:      c.PSK.Install.PSK,
		}, nil
	case "psk show":
		return &Options{
			Sub:         SubcommandPSKShow,
			DataDir:     c.PSK.Show.DataDir,
			MeshPSKFile: c.PSK.Show.MeshPSKFile,
		}, nil
	}

	// Default: run-mode (kong matches "run" or "" depending on
	// whether anything else was on the command line).
	r := &c.Run
	opts := &Options{
		Sub:                   SubcommandRun,
		Token:                 r.Token,
		DataDir:               r.DataDir,
		MeshPSKFile:           r.MeshPSKFile,
		MeshListen:            r.MeshListen,
		MeshPeers:             append([]string(nil), r.MeshPeers...),
		MeshAdvertise:         append([]string(nil), r.MeshAdvertise...),
		MeshDiscoveryLAN:      r.MeshDiscoveryLAN,
		MeshDiscoveryInterval: r.MeshDiscoveryInterval,
	}

	// --server takes precedence; --install-token's "host:port@..."
	// prefix fills the gap when --server isn't given. Lets a
	// copy-pasted token Just Work without an extra flag.
	if r.Server != "" {
		h, p, err := splitHostPort(r.Server)
		if err != nil {
			return opts, fmt.Errorf("--server: %w", err)
		}
		opts.RemoteHost, opts.RemotePort = h, p
	}
	if h, p, t, ok := splitTokenWithServer(opts.Token); ok {
		opts.Token = t
		if opts.RemoteHost == "" {
			opts.RemoteHost = h
			opts.RemotePort = p
		}
	}
	return opts, nil
}

// splitTokenWithServer parses a "host:port@<token>" prefix off the
// token string. Returns ok=false when no @ is present, leaving the
// token unchanged. Lets the install dialog ship a single
// copy-pasteable string without an explicit --server flag.
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
