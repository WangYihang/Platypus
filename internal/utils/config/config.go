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
//
// Anything not in this list is admin-UI policy (TTLs, channels,
// discovery toggles) and lives in the admin_settings DB table, not
// here. Bootstrap configuration vs runtime policy is a deliberate
// split — see internal/settings for the runtime side.
package config

import (
	"encoding/base64"
	"fmt"
	"net"
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

	DataDir string `name:"data-dir" env:"PLATYPUS_DATA_DIR" default:"./data" placeholder:"PATH" help:"Persistent state root: SQLite DB, recordings, release artifacts, optional TLS cert."`

	// --- Network ------------------------------------------------------

	Listen string `name:"listen" env:"PLATYPUS_LISTEN" placeholder:"HOST:PORT" help:"Bind address. Defaults to 0.0.0.0:<port-of-external-addr>."`

	// --- Mesh overlay -------------------------------------------------

	MeshProjectID string `name:"mesh-project" env:"PLATYPUS_MESH_PROJECT" default:"default" placeholder:"NAME" help:"Project ID under whose CA the mesh leaf is issued. Most deployments leave this at default."`

	// --- Sensitive / niche ------------------------------------------

	CAKEK string `name:"ca-kek" env:"PLATYPUS_CA_KEK" placeholder:"BASE64" help:"32-byte base64 CA private-key encryption key. When unset, a fresh KEK is auto-written under <data-dir>/ca.kek (NOT recommended for production: anyone with the data volume can read every CA private key)."`

	Dev bool `name:"dev" env:"PLATYPUS_DEV" help:"Development mode: relaxes CORS and a handful of cert-validation rules. Never enable in production."`
}

// PostParse fills derived defaults that depend on other fields.
// Called once after kong.Parse so the rest of the program sees a
// fully-resolved struct.
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
