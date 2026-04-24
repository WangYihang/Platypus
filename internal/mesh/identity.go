// Package mesh implements Platypus's self-organising agent overlay.
//
// Nodes (agents and the server) build a peer-to-peer mesh on top of
// TLS links. Every node has a project-CA-signed client certificate
// — NodeID is the "platypus://agent/<id>" URI SAN of that cert;
// gossip signatures are rooted in the cert's Ed25519 key; the Noise
// handshake runs over an X25519 keypair deterministically derived
// from the same Ed25519 seed via HKDF.
//
// A network-wide pre-shared key (PSK) is mixed into the Noise
// handshake for membership gating (Noise_XXpsk3). It complements
// cert-based identity — the cert proves WHO you are, the PSK
// proves you belong to this overlay.
package mesh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Identity bundles the Ed25519 keypair (for LSA / gossip signing)
// with the project-CA-signed leaf cert that anchors the NodeID,
// plus a derived X25519 keypair for the Noise handshake. The
// X25519 keys are deterministic from the Ed25519 seed via HKDF so
// an Identity restored from cert + key reproduces the same Noise
// static key every time. PrivateKey + X25519Private must never
// leave the process they were generated in.
type Identity struct {
	NodeID     string // "platypus://agent/<id>" SAN, extracted from CertPEM
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey

	X25519Private [32]byte
	X25519Public  [32]byte

	// CertPEM is the project-CA-signed leaf cert. Always non-empty
	// on a well-constructed Identity — it's the only way to mint
	// one (see LoadIdentityFromCert). Published via gossip so peers
	// can chain-verify membership against the project CA.
	CertPEM []byte
}

// x25519HKDFLabel domain-separates the X25519 derivation so the
// same Ed25519 seed cannot be accidentally reused to derive a key
// that chains into another protocol (e.g. file encryption). Kept
// stable across versions; if it ever needs to change, bump the
// suffix.
const x25519HKDFLabel = "platypus-mesh-noise-x25519-v1"

// deriveX25519Keypair derives a Curve25519 static keypair from an
// Ed25519 seed. The derivation is deterministic so the same seed
// reproduces the same X25519 keypair — which means persisting just
// the Ed25519 seed (inside the cert's PKCS#8 key) is enough to
// restore the full Identity.
//
// We can't safely convert Ed25519's signing key to a Curve25519
// Diffie-Hellman key in general (the conversion is defined but has
// subtle pitfalls around cross-protocol attacks), so HKDF-ing a
// fresh X25519 scalar from the seed with a domain-separation label
// avoids the hazard entirely.
func deriveX25519Keypair(seed []byte) ([32]byte, [32]byte, error) {
	var priv, pub [32]byte
	h := hkdf.New(sha256.New, seed, nil, []byte(x25519HKDFLabel))
	if _, err := io.ReadFull(h, priv[:]); err != nil {
		return priv, pub, fmt.Errorf("hkdf read: %w", err)
	}
	// Clamp per RFC 7748.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pubBytes, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return priv, pub, fmt.Errorf("curve25519 base mult: %w", err)
	}
	copy(pub[:], pubBytes)
	return priv, pub, nil
}

// fillX25519 computes and sets id's X25519 keypair from the Ed25519
// seed.
func (id *Identity) fillX25519() error {
	priv, pub, err := deriveX25519Keypair(id.PrivateKey.Seed())
	if err != nil {
		return fmt.Errorf("mesh: derive x25519: %w", err)
	}
	id.X25519Private = priv
	id.X25519Public = pub
	return nil
}

// LoadIdentityFromCert constructs an Identity from the PEM bytes
// of a project-CA-signed agent leaf cert + its PKCS#8 private key,
// as persisted by internal/agent.SaveIdentity. The NodeID is the
// cert's "platypus://agent/<id>" URI SAN, so mesh membership
// chains back to whoever issued the cert.
//
// Validation at load time:
//   - cert parses as an Ed25519 leaf with the expected SAN
//   - PKCS#8 key parses as an Ed25519 private key
//   - the private key's derived pubkey matches the cert's pubkey
//
// The returned Identity carries the raw certPEM in CertPEM so it
// can be republished via gossip for peers to verify against the
// project CA.
func LoadIdentityFromCert(certPEM, keyPEM []byte) (*Identity, error) {
	cert, err := parseAgentLeafCert(certPEM)
	if err != nil {
		return nil, err
	}
	agentID, err := agentIDFromCert(cert)
	if err != nil {
		return nil, err
	}
	certPub, err := ed25519PublicKeyFromCert(cert)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("mesh: no PEM block in key material")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("mesh: parse PKCS#8 key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("mesh: private key is %T, want ed25519", parsed)
	}
	derivedPub, ok := priv.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(derivedPub, certPub) {
		return nil, errors.New("mesh: private key does not match cert's public key")
	}

	id := &Identity{
		NodeID:     agentID,
		PublicKey:  certPub,
		PrivateKey: priv,
		CertPEM:    append([]byte(nil), certPEM...),
	}
	if err := id.fillX25519(); err != nil {
		return nil, err
	}
	return id, nil
}

// LoadOrCreatePSK reads a pre-shared key from file, or generates a
// new random 32-byte PSK if the file doesn't exist. Real
// deployments distribute the PSK out-of-band and every node points
// at the same bytes — it's the network-wide admission secret mixed
// into the Noise handshake.
func LoadOrCreatePSK(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("mesh: psk path is empty")
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		psk := decodePSK(raw)
		if len(psk) < 16 {
			return nil, fmt.Errorf("mesh: psk at %s is only %d bytes, need >= 16", path, len(psk))
		}
		return psk, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("mesh: read psk: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mesh: create psk dir: %w", err)
	}
	psk := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil {
		return nil, fmt.Errorf("mesh: generate psk: %w", err)
	}
	encoded := base32.StdEncoding.EncodeToString(psk) + "\n"
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("mesh: write psk: %w", err)
	}
	return psk, nil
}

// decodePSK accepts either raw bytes or a base32 textual form (we
// try base32 first because that's what LoadOrCreatePSK emits; fall
// back to the raw bytes if decoding fails). Whitespace is stripped.
func decodePSK(raw []byte) []byte {
	trimmed := strings.TrimSpace(string(raw))
	if decoded, err := base32.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) >= 16 {
		return decoded
	}
	return []byte(trimmed)
}
