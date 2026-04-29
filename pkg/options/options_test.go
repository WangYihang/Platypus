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
		EnvInstallToken, EnvServerAddr, EnvDataDir,
		EnvMeshPSK, EnvMeshPSKFile,
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

func TestParseArgs_PSKInstall(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"psk", "install", "ABCDEF1234"})
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
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"psk", "install", "--data-dir", "/srv/platypus", "AAAA"})
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
	clearAgentEnv(t)
	opts, err := parseArgs([]string{"psk", "show"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.Sub != SubcommandPSKShow {
		t.Fatalf("Sub = %v, want SubcommandPSKShow", opts.Sub)
	}
}

func TestParseArgs_PSKMissingVerb(t *testing.T) {
	clearAgentEnv(t)
	if _, err := parseArgs([]string{"psk"}); err == nil {
		t.Fatal("want error on `psk` with no verb")
	}
}

func TestParseArgs_PSKUnknownVerb(t *testing.T) {
	clearAgentEnv(t)
	if _, err := parseArgs([]string{"psk", "rotate", "abcd"}); err == nil {
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
// should be possible without any args. Kong's "default:withargs" on
// the run cmd lets the parse succeed when nothing's passed; main.go
// later checks whether persisted state exists.
func TestParseArgs_NoArgsAllowed(t *testing.T) {
	clearAgentEnv(t)
	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs([]) should succeed for re-run with persisted identity: %v", err)
	}
	if opts.Sub != SubcommandRun {
		t.Fatalf("Sub = %v, want SubcommandRun", opts.Sub)
	}
	if opts.Token != "" {
		t.Fatalf("Token unexpectedly populated: %q", opts.Token)
	}
}
