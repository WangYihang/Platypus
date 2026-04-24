package agent

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Identity bundles the three PEM+key artefacts a v2 agent needs to
// dial the server: its private key, its client certificate (signed
// by the project CA during enrollment), and the project CA itself
// (used to verify the server's TLS chain).
type Identity struct {
	PrivateKey ed25519.PrivateKey
	CertPEM    []byte
	CAPEM      []byte
}

// ErrIdentityNotFound is returned by LoadIdentity when any of the
// three persisted files (private key, client cert, project CA) is
// missing. Callers treat this as "not enrolled yet" and kick off
// the enrollment flow.
var ErrIdentityNotFound = errors.New("agent: identity not found on disk")

// identityFileNames maps the on-disk basenames to their purpose;
// keeping them in one place so tests and production agree.
const (
	keyFileName = "client.key"
	crtFileName = "client.crt"
	caFileName  = "project_ca.crt"
)

// SaveIdentity writes the private key as a PKCS#8 PEM block plus the
// two certificate PEMs into dir. All three files land with mode
// 0600; the directory is created with 0700 if it doesn't exist.
func SaveIdentity(dir string, priv ed25519.PrivateKey, certPEM, caPEM []byte) error {
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("agent: SaveIdentity: private key wrong length %d", len(priv))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agent: SaveIdentity mkdir %s: %w", dir, err)
	}

	// Marshal the Ed25519 key through PKCS#8 so the on-disk format
	// matches what `openssl` and friends expect; round-tripping via
	// a raw 64-byte blob works but produces files nothing else can
	// read.
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("agent: SaveIdentity marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	for _, e := range []struct {
		name  string
		bytes []byte
	}{
		{keyFileName, keyPEM},
		{crtFileName, certPEM},
		{caFileName, caPEM},
	} {
		if err := os.WriteFile(filepath.Join(dir, e.name), e.bytes, 0o600); err != nil {
			return fmt.Errorf("agent: SaveIdentity write %s: %w", e.name, err)
		}
	}
	return nil
}

// LoadIdentity reads the three files back. Returns ErrIdentityNotFound
// when any of them are missing; returns a wrapped parse error for
// malformed contents.
func LoadIdentity(dir string) (*Identity, error) {
	keyPath := filepath.Join(dir, keyFileName)
	crtPath := filepath.Join(dir, crtFileName)
	caPath := filepath.Join(dir, caFileName)

	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("agent: LoadIdentity read key: %w", err)
	}
	crtBytes, err := os.ReadFile(crtPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("agent: LoadIdentity read cert: %w", err)
	}
	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIdentityNotFound
		}
		return nil, fmt.Errorf("agent: LoadIdentity read ca: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("agent: LoadIdentity: private key PEM decode failed")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: LoadIdentity parse key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("agent: LoadIdentity: private key type %T is not Ed25519", parsed)
	}

	return &Identity{
		PrivateKey: priv,
		CertPEM:    crtBytes,
		CAPEM:      caBytes,
	}, nil
}
