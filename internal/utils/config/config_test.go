package config_test

import (
	"errors"
	"net"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

// parseFromEnv runs kong against a dummy argv ([] = no flags) with
// the supplied env, returning the populated Options + any error from
// PostParse. Mirrors what cmd/platypus-server/main.go does, minus
// the os.Exit on parse failure.
func parseFromEnv(t *testing.T, env map[string]string) (*config.Options, error) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	var opts config.Options
	parser, err := kong.New(&opts, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse(nil); err != nil {
		return &opts, err
	}
	if err := opts.PostParse(); err != nil {
		return &opts, err
	}
	return &opts, nil
}

// L0: external_addr is the only required field. A blank env should
// fail at parse time so operators see the missing value loudly
// instead of booting with an empty cert SAN.
func TestParseRequiresExternalAddr(t *testing.T) {
	_, err := parseFromEnv(t, nil)
	if err == nil {
		t.Fatal("parse with no env should fail; PLATYPUS_EXTERNAL_ADDR is required")
	}
}

// L1: minimal happy path — only external_addr set. Listen and
// data_dir come from defaults / derivations.
func TestParseMinimal(t *testing.T) {
	opts, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "203.0.113.10:443",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.ExternalAddr != "203.0.113.10:443" {
		t.Errorf("ExternalAddr = %q", opts.ExternalAddr)
	}
	if opts.DataDir != "./data" {
		t.Errorf("DataDir = %q; want ./data", opts.DataDir)
	}
	// Listen derives from external port when unset — same port both sides.
	wantListen := "0.0.0.0:443"
	if opts.Listen != wantListen {
		t.Errorf("Listen = %q; want %q", opts.Listen, wantListen)
	}
}

// L1: explicit --listen overrides the derive-from-external-port
// logic, so an operator behind a reverse proxy can bind 127.0.0.1:8000
// while announcing the public port to agents.
func TestParseListenOverride(t *testing.T) {
	opts, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "203.0.113.10:443",
		"PLATYPUS_LISTEN":        "127.0.0.1:8000",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Listen != "127.0.0.1:8000" {
		t.Errorf("Listen = %q; want 127.0.0.1:8000", opts.Listen)
	}
}

// L2: derived paths all live under data_dir.
func TestDerivedPaths(t *testing.T) {
	opts, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "x:443",
		"PLATYPUS_DATA_DIR":      "/var/lib/platypus",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cases := []struct {
		got, want string
	}{
		{opts.DBPath(), "/var/lib/platypus/platypus.db"},
		{opts.RecordingDir(), "/var/lib/platypus/recordings"},
		{opts.MeshPSKPath(), "/var/lib/platypus/mesh.psk"},
		{opts.CertPath(), "/var/lib/platypus/cert.pem"},
		{opts.KeyPath(), "/var/lib/platypus/key.pem"},
		{opts.CAKEKPath(), "/var/lib/platypus/ca.kek"},
		{opts.ReleasesDir(), "/var/lib/platypus/releases"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q; want %q", c.got, c.want)
		}
	}
}

// L2: external_addr without a port is rejected. Without a port the
// derived listen would be malformed and the cert SAN logic would
// have to guess — fail loudly instead.
func TestExternalAddrMustBeHostPort(t *testing.T) {
	_, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "no-port-here",
	})
	if err == nil {
		t.Fatal("external_addr without :port should fail")
	}
	var addrErr *net.AddrError
	if errors.As(err, &addrErr) {
		// fine
		return
	}
}
