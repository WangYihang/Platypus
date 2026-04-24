package agent

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// ProjectCAEnvVar is the name of the environment variable the install
// script populates with the project's trust anchor. The value is a
// base64-encoded PEM CERTIFICATE block; agents load it on startup to
// pin the server's TLS chain (killing InsecureSkipVerify on the
// agent→server hop).
const ProjectCAEnvVar = "PLATYPUS_PROJECT_CA"

// Sentinel errors returned by LoadProjectCA. Callers (the dialer,
// logging) branch on these to produce useful messages without having
// to string-match.
var (
	ErrProjectCABadBase64 = errors.New("agent: PLATYPUS_PROJECT_CA is not valid base64")
	ErrProjectCABadPEM    = errors.New("agent: PLATYPUS_PROJECT_CA does not contain a CERTIFICATE PEM block")
	ErrProjectCABadCert   = errors.New("agent: PLATYPUS_PROJECT_CA PEM is not a parseable x509 certificate")
)

// LoadProjectCA parses the base64-wrapped PEM value the install
// script sets in PLATYPUS_PROJECT_CA. Returns a fresh *x509.CertPool
// containing only that anchor, or (nil, nil) when the env var is
// empty (legacy installs that predate the v2 enroll flow).
//
// The caller is expected to read os.Getenv("PLATYPUS_PROJECT_CA")
// and pass it in; tests inject the value directly to avoid t.Setenv
// gymnastics.
func LoadProjectCA(envValue string) (*x509.CertPool, error) {
	if envValue == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(envValue)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProjectCABadBase64, err)
	}

	block, _ := pem.Decode(decoded)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, ErrProjectCABadPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProjectCABadCert, err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool, nil
}
