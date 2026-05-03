// Package options is the platypus-agent command-line surface.
// Parsing runs through alecthomas/kong with the standard resolution
// chain: explicit flag → env var → default value (struct-tag).
//
// On top of that, main.go overlays a third source — persisted state
// on disk — so an agent restart picks up the server endpoint embedded
// in the cert (and similar invariants stamped by the previous
// bootstrap). That layer lives in main.go, not here.
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

// Options is the parsed CLI surface used by main.go.
type Options struct {
	Token      string
	RemoteHost string
	RemotePort int
	DataDir    string

	// BaselinePluginIDs is the operator-chosen system-plugin
	// allowlist for first boot. Empty slice = "no allowlist", which
	// the agent interprets as "install only the mandatory core
	// (sys-info)". Persisted on first boot to baseline.json so
	// later boots reproduce the same selection without the flag.
	BaselinePluginIDs []string

	MeshListen            string
	MeshPeers             []string
	MeshAdvertise         []string
	MeshDiscoveryLAN      bool
	MeshDiscoveryInterval int
}

// Env-var names exposed for tests / docs / install scripts.
const (
	EnvInstallToken     = "PLATYPUS_INSTALL_TOKEN"
	EnvServerAddr       = "PLATYPUS_SERVER"
	EnvDataDir          = "PLATYPUS_DATA_DIR"
	EnvBaselinePlugins  = "PLATYPUS_BASELINE_PLUGINS"
)

var (
	ErrMissingToken  = errors.New("install token required")
	ErrMissingServer = errors.New("server address required")
)

type cli struct {
	Token string `arg:"" optional:"" name:"install-token" env:"PLATYPUS_INSTALL_TOKEN" help:"Install token (PAT) to redeem on first run; ignored after enrollment. May embed server as 'host:port@<token>', or be a 'pinst_' install bundle."`

	Server string `name:"server" env:"PLATYPUS_SERVER" placeholder:"HOST:PORT" help:"Server endpoint. Falls back to the embedded prefix in --install-token (when present) or persisted state."`

	DataDir string `name:"data-dir" env:"PLATYPUS_DATA_DIR" placeholder:"PATH" help:"Persistent state root. Defaults to ~/.platypus/agent (or /var/lib/platypus when running as root)."`

	BaselinePlugins string `name:"baseline-plugins" env:"PLATYPUS_BASELINE_PLUGINS" placeholder:"id1,id2,..." help:"Comma-separated allowlist of system plugin ids to install on first boot. Empty = install only the mandatory core (sys-info). Ignored on later boots once baseline.json is persisted."`

	MeshListen            string   `name:"mesh-listen" placeholder:"HOST:PORT" help:"Address to accept inbound mesh links from peers (NAT-relay / hub-and-spoke fan-out). Empty = leaf-only (the typical configuration)."`
	MeshDiscoveryLAN      bool     `name:"mesh-discovery" default:"true" negatable:"" help:"Enable mDNS LAN discovery."`
	MeshDiscoveryInterval int      `name:"mesh-discovery-interval" default:"30" placeholder:"SECS" help:"mDNS scan interval (seconds)."`
	MeshAdvertise         []string `name:"mesh-advertise" placeholder:"HOST:PORT" help:"Override advertised mesh listen address(es). Repeatable."`
	MeshPeers             []string `name:"peers" placeholder:"HOST:PORT" help:"Explicit mesh bootstrap peer. Repeatable."`

	Version kong.VersionFlag `short:"v" help:"Print version and exit."`
}

// InitOptions parses os.Args and returns a populated Options. Kong
// prints its own usage on parse failure and exits; the only errors
// returned through here are the missing-token / missing-server cases
// main handles with a contextual message.
func InitOptions() (*Options, error) {
	return parseArgs(os.Args[1:])
}

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
	if _, err := parser.Parse(argv); err != nil {
		return nil, err
	}

	opts := &Options{
		Token:                 c.Token,
		DataDir:               c.DataDir,
		BaselinePluginIDs:     parseBaselinePlugins(c.BaselinePlugins),
		MeshListen:            c.MeshListen,
		MeshPeers:             append([]string(nil), c.MeshPeers...),
		MeshAdvertise:         append([]string(nil), c.MeshAdvertise...),
		MeshDiscoveryLAN:      c.MeshDiscoveryLAN,
		MeshDiscoveryInterval: c.MeshDiscoveryInterval,
	}

	// --server takes precedence; --install-token's "host:port@..."
	// prefix fills the gap when --server isn't given. Lets a
	// copy-pasted token Just Work without an extra flag.
	if c.Server != "" {
		h, p, err := splitHostPort(c.Server)
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

// parseBaselinePlugins splits a comma-separated allowlist into a
// trimmed, deduplicated string slice. Empty input returns nil so the
// agent can distinguish "no allowlist passed" (mandatory core only)
// from "explicit empty allowlist" (functionally identical, but the
// nil form keeps debug logs cleaner).
func parseBaselinePlugins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
