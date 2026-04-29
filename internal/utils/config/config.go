// Package config is the platypus-server configuration surface.
//
// All settings are exposed as both --kebab-case CLI flags and
// PLATYPUS_<UPPER_SNAKE> environment variables. The kong parser
// handles both, with --flag taking precedence over env. There is no
// YAML / TOML / INI config file — operators set 1-3 env vars (or
// flags) and rely on file conventions inside data_dir for the rest.
//
// File conventions (everything under <data_dir>/):
//
//	platypus.db                — SQLite store (auto)
//	ca.kek                     — CA key-encryption-key fallback (auto)
//	recordings/                — terminal session captures (auto)
//	releases/<channel>/...     — agent release artifacts (operator drops in)
//	cert.pem + key.pem         — custom TLS leaf (optional; otherwise self-signed)
//	mesh.psk                   — mesh PSK (optional; presence enables overlay)
//
// Anything not in this list is admin-UI policy (TTLs, channels,
// discovery toggles) and lives in the admin_settings DB table, not
// here. Bootstrap configuration vs runtime policy is a deliberate
// split — see internal/settings for the runtime side.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Options is the canonical server configuration surface, populated
// once at startup by kong.Parse(&Options{}). All consumers receive a
// pointer to this; nothing else reads env / args directly.
//
// Field order is "what an operator typically sets first" → "rare
// overrides" so the --help output reads top-down.
type Options struct {
	// --- Required / commonly set --------------------------------------

	ExternalAddr string `name:"external-addr" env:"PLATYPUS_EXTERNAL_ADDR" required:"" placeholder:"HOST:PORT" help:"Address agents reach (drives TLS cert SAN). For deployments behind NAT or DNS this differs from --listen."`

	DataDir string `name:"data-dir" env:"PLATYPUS_DATA_DIR" default:"./data" placeholder:"PATH" help:"Persistent state root: SQLite DB, recordings, release artifacts, mesh PSK, optional TLS cert."`

	// --- Network ------------------------------------------------------

	Listen string `name:"listen" env:"PLATYPUS_LISTEN" placeholder:"HOST:PORT" help:"Bind address. Defaults to 0.0.0.0:<port-of-external-addr>."`

	// --- Mesh overlay -------------------------------------------------

	MeshProjectID string `name:"mesh-project" env:"PLATYPUS_MESH_PROJECT" default:"default" placeholder:"NAME" help:"Project ID under whose CA the mesh leaf is issued. Most deployments leave this at default."`

	// --- Distributor (S3-compatible release backend; optional) -------
	//
	// Setting --s3-endpoint enables the distributor. When empty the
	// release-artifact endpoints return 503 — agent self-upgrade is
	// disabled but everything else keeps working.

	S3Endpoint        string `name:"s3-endpoint"          env:"PLATYPUS_S3_ENDPOINT"          placeholder:"HOST:PORT" help:"S3-compatible endpoint hosting agent releases. Empty disables the distributor."`
	S3Region          string `name:"s3-region"            env:"PLATYPUS_S3_REGION"            default:"us-east-1" help:"S3 region; MinIO usually accepts any value."`
	S3Bucket          string `name:"s3-bucket"            env:"PLATYPUS_S3_BUCKET"            placeholder:"NAME" help:"S3 bucket holding the release manifest + binaries."`
	S3Prefix          string `name:"s3-prefix"            env:"PLATYPUS_S3_PREFIX"            default:"agent/" help:"Object key prefix; manifests at <prefix>manifest/<channel>.json."`
	S3AccessKeyID     string `name:"s3-access-key-id"     env:"PLATYPUS_S3_ACCESS_KEY_ID"     help:"S3 access key ID. Use --s3-access-key-id-file (or *_FILE env) to read from a Docker/k8s secret file."`
	S3AccessKeyIDFile string `name:"s3-access-key-id-file" env:"PLATYPUS_S3_ACCESS_KEY_ID_FILE" type:"existingfile" placeholder:"PATH" help:"Path to a file containing the S3 access key ID. Higher precedence than --s3-access-key-id."`
	S3SecretKey       string `name:"s3-secret-key"        env:"PLATYPUS_S3_SECRET_ACCESS_KEY"        help:"S3 secret access key. Use --s3-secret-key-file (or *_FILE env) for secret files."`
	S3SecretKeyFile   string `name:"s3-secret-key-file"   env:"PLATYPUS_S3_SECRET_ACCESS_KEY_FILE"   type:"existingfile" placeholder:"PATH" help:"Path to a file containing the S3 secret access key."`
	S3Secure          bool   `name:"s3-secure"            env:"PLATYPUS_S3_SECURE"             default:"true" negatable:"" help:"Use TLS to talk to the S3 endpoint. Set --no-s3-secure for plain HTTP MinIO in dev."`

	// --- Sensitive / niche ------------------------------------------

	CAKEK string `name:"ca-kek" env:"PLATYPUS_CA_KEK" placeholder:"BASE64" help:"32-byte base64 CA private-key encryption key. When unset, a fresh KEK is auto-written under <data-dir>/ca.kek (NOT recommended for production: anyone with the data volume can read every CA private key)."`

	Dev bool `name:"dev" env:"PLATYPUS_DEV" help:"Development mode: relaxes CORS and a handful of cert-validation rules. Never enable in production."`
}

// PostParse fills derived defaults that depend on other fields and
// loads file-backed secrets. Called once after kong.Parse so the rest
// of the program sees a fully-resolved struct.
func (o *Options) PostParse() error {
	// --- Listen defaults to 0.0.0.0:<external port>. Operators behind
	// a reverse proxy (where external port != bind port) set --listen
	// explicitly; everyone else gets the same port on both sides.
	if o.Listen == "" {
		_, port, err := net.SplitHostPort(o.ExternalAddr)
		if err != nil {
			return fmt.Errorf("--external-addr must be host:port: %w", err)
		}
		o.Listen = "0.0.0.0:" + port
	}

	// --- Secret files override inline values (so a --s3-secret-key
	// inherited from a global default doesn't shadow a per-deployment
	// secret file). 12-factor pattern: secrets are paths, code reads
	// the path lazily at startup.
	if o.S3AccessKeyIDFile != "" {
		v, err := readSecretFile(o.S3AccessKeyIDFile)
		if err != nil {
			return fmt.Errorf("--s3-access-key-id-file: %w", err)
		}
		o.S3AccessKeyID = v
	}
	if o.S3SecretKeyFile != "" {
		v, err := readSecretFile(o.S3SecretKeyFile)
		if err != nil {
			return fmt.Errorf("--s3-secret-key-file: %w", err)
		}
		o.S3SecretKey = v
	}

	return o.validate()
}

// validate enforces post-load invariants kong's own tag-based rules
// don't catch. Currently:
//
//   - When --s3-endpoint is set the credentials must also be present
//     (otherwise the artifact handler boots fine but every fetch
//     401s, and operators waste an hour figuring out why).
func (o *Options) validate() error {
	if o.S3Endpoint == "" {
		return nil
	}
	if o.S3AccessKeyID == "" || o.S3SecretKey == "" {
		return ErrMissingS3Credentials
	}
	if o.S3Bucket == "" {
		return errors.New("--s3-bucket is required when --s3-endpoint is set")
	}
	return nil
}

// CAKEKBytes returns the decoded KEK or nil when no KEK was supplied.
// Caller treats nil as "use the auto-generated kek file under
// <data_dir>/ca.kek".
func (o *Options) CAKEKBytes() ([]byte, error) {
	if o.CAKEK == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(o.CAKEK)
}

// DBPath is the SQLite database path: always <data_dir>/platypus.db.
// Centralised so call sites don't accidentally derive different paths.
func (o *Options) DBPath() string { return joinPath(o.DataDir, "platypus.db") }

// RecordingDir is the directory under which terminal session
// recordings are written. Always <data_dir>/recordings.
func (o *Options) RecordingDir() string { return joinPath(o.DataDir, "recordings") }

// MeshPSKPath is the conventional mesh PSK location. Mesh is enabled
// when this file exists at server startup; otherwise the mesh
// subsystem stays inert.
func (o *Options) MeshPSKPath() string { return joinPath(o.DataDir, "mesh.psk") }

// CertPath / KeyPath name the optional custom TLS leaf locations.
// When both files exist at startup the ingress uses them; otherwise
// it self-issues a leaf from the project CA.
func (o *Options) CertPath() string { return joinPath(o.DataDir, "cert.pem") }
func (o *Options) KeyPath() string  { return joinPath(o.DataDir, "key.pem") }

// CAKEKPath is the on-disk fallback for the KEK. When --ca-kek /
// PLATYPUS_CA_KEK isn't set, the server reads (or auto-generates) the
// KEK from this path.
func (o *Options) CAKEKPath() string { return joinPath(o.DataDir, "ca.kek") }

// ReleasesDir is the on-disk root for agent release artifacts. Used
// by the future local-file distributor (commit 3); the current S3
// distributor doesn't read from here.
func (o *Options) ReleasesDir() string { return joinPath(o.DataDir, "releases") }

// ErrMissingS3Credentials is returned by Validate when --s3-endpoint
// is set but either of the credential fields is empty. Letting the
// server boot with blank creds and a configured endpoint masks
// deployment errors — fail loudly at startup instead.
var ErrMissingS3Credentials = errors.New(
	"--s3-endpoint is set but credentials are missing: provide " +
		"--s3-access-key-id + --s3-secret-key (or PLATYPUS_S3_ACCESS_KEY_ID + " +
		"PLATYPUS_S3_SECRET_ACCESS_KEY env vars, or *_FILE variants pointing " +
		"at a Docker/k8s secret file)",
)

// readSecretFile loads a single-line secret from a path, trimming
// trailing whitespace/newlines that text-mode editors and
// docker secrets sometimes append.
func readSecretFile(path string) (string, error) {
	b, err := readFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), " \t\r\n"), nil
}
