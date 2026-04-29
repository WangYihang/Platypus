package options

import (
	"testing"
)

// noEnv produces a stub for the env lookup function so tests don't
// touch the real process environment.
func noEnv(string) string { return "" }

// envOf builds an env stub from a fixed map.
func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseArgs_RunWithPositionalToken(t *testing.T) {
	opts, err := parseArgs([]string{"--host", "1.2.3.4", "--port", "9443", "plt_abc.def"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Sub != SubcommandRun {
		t.Fatalf("Sub = %v, want SubcommandRun", opts.Sub)
	}
	if opts.Token != "plt_abc.def" {
		t.Fatalf("Token = %q", opts.Token)
	}
	if opts.RemoteHost != "1.2.3.4" || opts.RemotePort != 9443 {
		t.Fatalf("server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

func TestParseArgs_TokenFromEnv(t *testing.T) {
	opts, err := parseArgs([]string{"--host", "h", "--port", "1"}, envOf(map[string]string{
		EnvInstallToken: "plt_from_env.deadbeef",
	}))
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Token != "plt_from_env.deadbeef" {
		t.Fatalf("Token = %q", opts.Token)
	}
}

func TestParseArgs_ServerFromEnv(t *testing.T) {
	opts, err := parseArgs([]string{"plt_x.y"}, envOf(map[string]string{
		EnvServerAddr: "10.0.0.1:13337",
	}))
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.RemoteHost != "10.0.0.1" || opts.RemotePort != 13337 {
		t.Fatalf("server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

// TestParseArgs_ServerFromTokenPrefix exercises the `host:port@token`
// shape — admins can paste one string from the install dialog and
// not bother with --host/--port.
func TestParseArgs_ServerFromTokenPrefix(t *testing.T) {
	opts, err := parseArgs([]string{"server.corp:9443@plt_abc.def"}, noEnv)
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

// TestParseArgs_FlagOverridesTokenPrefix: the explicit --host wins
// over whatever the token's prefix says.
func TestParseArgs_FlagOverridesTokenPrefix(t *testing.T) {
	opts, err := parseArgs([]string{"--host", "override", "--port", "1234", "tok-server:9443@plt_a.b"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.RemoteHost != "override" || opts.RemotePort != 1234 {
		t.Fatalf("flag override lost: server = %s:%d", opts.RemoteHost, opts.RemotePort)
	}
}

func TestParseArgs_ExtraPositional(t *testing.T) {
	if _, err := parseArgs([]string{"plt_a.b", "garbage"}, noEnv); err == nil {
		t.Fatal("want error on extra positional, got nil")
	}
}

func TestParseArgs_PSKInstall(t *testing.T) {
	opts, err := parseArgs([]string{"psk", "install", "ABCDEF1234"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Sub != SubcommandPSKInstall {
		t.Fatalf("Sub = %v, want SubcommandPSKInstall", opts.Sub)
	}
	if opts.PSKArg != "ABCDEF1234" {
		t.Fatalf("PSKArg = %q", opts.PSKArg)
	}
}

func TestParseArgs_PSKInstall_DataDirFlag(t *testing.T) {
	opts, err := parseArgs([]string{"psk", "install", "--data-dir", "/srv/platypus", "AAAA"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.DataDir != "/srv/platypus" {
		t.Fatalf("DataDir = %q", opts.DataDir)
	}
	if opts.PSKArg != "AAAA" {
		t.Fatalf("PSKArg = %q", opts.PSKArg)
	}
}

func TestParseArgs_PSKShow(t *testing.T) {
	opts, err := parseArgs([]string{"psk", "show"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Sub != SubcommandPSKShow {
		t.Fatalf("Sub = %v, want SubcommandPSKShow", opts.Sub)
	}
}

func TestParseArgs_PSKMissingVerb(t *testing.T) {
	if _, err := parseArgs([]string{"psk"}, noEnv); err == nil {
		t.Fatal("want error on `psk` with no verb")
	}
}

func TestParseArgs_PSKUnknownVerb(t *testing.T) {
	if _, err := parseArgs([]string{"psk", "rotate", "abcd"}, noEnv); err == nil {
		t.Fatal("want error on unknown psk verb")
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
	opts, err := parseArgs([]string{"--peers", "a:1", "--peers", "b:2", "plt_x.y"}, noEnv)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if len(opts.MeshPeers) != 2 || opts.MeshPeers[0] != "a:1" || opts.MeshPeers[1] != "b:2" {
		t.Fatalf("MeshPeers = %v", opts.MeshPeers)
	}
}
