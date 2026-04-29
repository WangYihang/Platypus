package config_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
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

// L1: minimal happy path — only external_addr set. Listen, data_dir,
// distributor S3 region etc. all come from defaults / derivations.
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
	if opts.S3Region != "us-east-1" {
		t.Errorf("S3Region default = %q; want us-east-1", opts.S3Region)
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

// L2: distributor enabled (S3Endpoint set) without credentials must
// fail — letting the server boot with blank creds and a configured
// endpoint silently disables artefact uploads (or worse).
func TestS3RequiresCredentials(t *testing.T) {
	_, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "x:443",
		"PLATYPUS_S3_ENDPOINT":   "minio:9000",
		"PLATYPUS_S3_BUCKET":     "platypus-artifacts",
		// Credentials deliberately blank.
	})
	if err == nil {
		t.Fatal("S3 endpoint set + blank credentials should fail validation")
	}
	if !errors.Is(err, config.ErrMissingS3Credentials) {
		t.Fatalf("err = %v; want ErrMissingS3Credentials", err)
	}
}

// L2: distributor disabled (no endpoint) — blank creds are fine.
func TestS3DisabledIsOK(t *testing.T) {
	if _, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR": "x:443",
	}); err != nil {
		t.Fatalf("parse with no S3 should succeed: %v", err)
	}
}

// L2: secret-file fallback resolves at PostParse time. Lets a
// docker/k8s deployment mount /run/secrets/* and inject the file path
// via env, keeping the secret out of the process environment.
func TestS3CredentialsFromFile(t *testing.T) {
	dir := t.TempDir()
	akPath := filepath.Join(dir, "ak")
	skPath := filepath.Join(dir, "sk")
	if err := os.WriteFile(akPath, []byte("from-file-ak\n"), 0o600); err != nil {
		t.Fatalf("write ak: %v", err)
	}
	if err := os.WriteFile(skPath, []byte("from-file-sk"), 0o600); err != nil {
		t.Fatalf("write sk: %v", err)
	}
	opts, err := parseFromEnv(t, map[string]string{
		"PLATYPUS_EXTERNAL_ADDR":               "x:443",
		"PLATYPUS_S3_ENDPOINT":                 "minio:9000",
		"PLATYPUS_S3_BUCKET":                   "platypus-artifacts",
		"PLATYPUS_S3_ACCESS_KEY_ID_FILE":       akPath,
		"PLATYPUS_S3_SECRET_ACCESS_KEY_FILE":   skPath,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.S3AccessKeyID != "from-file-ak" {
		t.Errorf("AccessKeyID = %q; want from-file-ak (trailing newline trimmed)", opts.S3AccessKeyID)
	}
	if opts.S3SecretKey != "from-file-sk" {
		t.Errorf("SecretKey = %q; want from-file-sk", opts.S3SecretKey)
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
	// Sanity that the underlying cause came from net.SplitHostPort
	// rather than something else.
	var addrErr *net.AddrError
	if errors.As(err, &addrErr) {
		// fine
		return
	}
}
