package options

import (
	"testing"
)

// clearAgentEnv unsets every agent-side env var the tests care about,
// so a CI runner with PLATYPUS_INSTALL_TOKEN exported in its
// environment can't poison a test that means to verify "no env at
// all". t.Setenv with empty string puts the var in the process env
// as "" rather than unsetting it, which is the desired behaviour
// here — kong reads "" as not-set.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvInstallToken, EnvServerAddr, EnvDataDir, EnvBaselinePlugins,
	} {
		t.Setenv(k, "")
	}
}

func TestParseArgs_RunWithPositionalToken(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"--server", "1.2.3.4:9443", "plt_abc.def"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Token != "plt_abc.def" {
		t.Fatalf("Token = %q", opts.Token)
	}
	if opts.RemoteHost != "1.2.3.4" || opts.RemotePort != 9443 {
		t.Fatalf("server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

func TestParseArgs_TokenFromEnv(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv(EnvInstallToken, "plt_from_env.deadbeef")
	opts, err := parseArgs([]string{"--server", "h:1"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Token != "plt_from_env.deadbeef" {
		t.Fatalf("Token = %q", opts.Token)
	}
}

func TestParseArgs_ServerFromEnv(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv(EnvServerAddr, "10.0.0.1:13337")
	opts, err := parseArgs([]string{"plt_x.y"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.RemoteHost != "10.0.0.1" || opts.RemotePort != 13337 {
		t.Fatalf("server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

// TestParseArgs_ServerFromTokenPrefix exercises the `host:port@token`
// shape — admins can paste one string from the install dialog and
// not bother with --server.
func TestParseArgs_ServerFromTokenPrefix(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"server.corp:9443@plt_abc.def"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.RemoteHost != "server.corp" || opts.RemotePort != 9443 {
		t.Fatalf("server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
	if opts.Token != "plt_abc.def" {
		t.Fatalf("Token = %q (should not include the host:port prefix)", opts.Token)
	}
}

// TestParseArgs_FlagOverridesTokenPrefix: explicit --server wins
// over whatever the token's prefix says.
func TestParseArgs_FlagOverridesTokenPrefix(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"--server", "override:1234", "tok-server:9443@plt_a.b"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.RemoteHost != "override" || opts.RemotePort != 1234 {
		t.Fatalf("flag override lost: server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

func TestParseArgs_ExtraPositional(t *testing.T) {
	clearAgentEnv(t)
	if _, err := parseArgs([]string{"plt_a.b", "garbage"}); err == nil {
		t.Fatal("want error on extra positional, got nil")
	}
}

// TestSplitTokenWithServer covers the parser helper directly: a
// well-formed prefix splits cleanly, a missing @ leaves the token
// untouched, and a malformed prefix degrades to the same.
func TestSplitTokenWithServer(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		h, p, tok, ok := splitTokenWithServer("h:8080@plt_x.y")
		if !ok || h != "h" || p != 8080 || tok != "plt_x.y" {
			t.Fatalf("h=%s p=%d tok=%q ok=%v", h, p, tok, ok)
		}
	})
	t.Run("no @", func(t *testing.T) {
		h, p, tok, ok := splitTokenWithServer("plt_x.y")
		if ok || h != "" || p != 0 || tok != "plt_x.y" {
			t.Fatalf("got h=%s p=%d tok=%q ok=%v", h, p, tok, ok)
		}
	})
	t.Run("malformed prefix", func(t *testing.T) {
		// Missing port — the parser shouldn't strip the @ in this
		// case because we can't safely use the prefix.
		h, p, tok, ok := splitTokenWithServer("hostonly@plt_x.y")
		if ok || tok != "hostonly@plt_x.y" {
			t.Fatalf("malformed should fall through: tok=%q ok=%v h=%s p=%d", tok, ok, h, p)
		}
	})
}

// TestParseArgs_RepeatablePeers exercises the --peers slice flag.
func TestParseArgs_RepeatablePeers(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"--peers", "a:1", "--peers", "b:2", "plt_x.y"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if len(opts.MeshPeers) != 2 || opts.MeshPeers[0] != "a:1" || opts.MeshPeers[1] != "b:2" {
		t.Fatalf("MeshPeers = %v", opts.MeshPeers)
	}
}

// TestParseArgs_NoArgsAllowed: a re-run with persisted identity
// should be possible without any args.
// TestParseArgs_BaselinePlugins ensures the --baseline-plugins flag
// (and its env equivalent) round-trip through to opts as a clean,
// trimmed, deduplicated string slice. The default (no flag, no env)
// must produce nil so the agent's first-boot logic can distinguish
// "operator never set a baseline" from "operator set an empty one".
func TestParseArgs_BaselinePlugins(t *testing.T) {
	clearAgentEnv(t)
	cases := []struct {
		name string
		argv []string
		env  string
		want []string
	}{
		{name: "default", argv: []string{"--server", "h:1", "plt_a.b"}, want: nil},
		{name: "empty flag", argv: []string{"--server", "h:1", "--baseline-plugins", "", "plt_a.b"}, want: nil},
		{name: "single", argv: []string{"--server", "h:1", "--baseline-plugins", "com.platypus.sys-info", "plt_a.b"}, want: []string{"com.platypus.sys-info"}},
		{name: "multi + dedup + trim", argv: []string{"--server", "h:1", "--baseline-plugins", " a , b,a ,, c ", "plt_a.b"}, want: []string{"a", "b", "c"}},
		{name: "via env", argv: []string{"--server", "h:1", "plt_a.b"}, env: "x,y", want: []string{"x", "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			if tc.env != "" {
				t.Setenv(EnvBaselinePlugins, tc.env)
			}
			opts, err := parseArgs(tc.argv)
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			if !equalStrings(opts.BaselinePluginIDs, tc.want) {
				t.Fatalf("BaselinePluginIDs = %v; want %v", opts.BaselinePluginIDs, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseArgs_NoArgsAllowed(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs([]) should succeed for re-run with persisted identity: %v", err)
	}
	if opts.Token != "" {
		t.Fatalf("Token unexpectedly populated: %q", opts.Token)
	}
}
