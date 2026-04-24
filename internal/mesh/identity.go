// Package mesh implements Platypus's self-organising agent overlay.
//
// Nodes (agents and the server) build a peer-to-peer mesh on top of TLS
// links. Each node has an Ed25519 long-term identity keypair; its NodeID
// is derived from the public key and is self-certifying — anyone claiming
// a NodeID must be able to sign with the matching private key.
//
// A network-wide pre-shared key (PSK) gates membership. Discovery,
// link-state routing, and payload forwarding are all implemented on top
// of the existing length-prefixed protobuf Envelope transport.
package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// NodeIDLen is the encoded length of a NodeID in characters. base32 over
// a 32-byte SHA-256 digest yields 52 characters; we truncate to the first
// 32 for readability — 160 bits of preimage resistance is still well
// above any practical collision horizon.
const NodeIDLen = 32

// nodeIDEncoding is unpadded base32 using the standard alphabet, then
// lower-cased. Same transform as Yggdrasil / Tor onion v3 style IDs so
// NodeIDs are URL-safe and case-insensitive.
var nodeIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Identity bundles an Ed25519 keypair (for NodeID derivation +
// LSA/gossip signing) with a derived X25519 keypair (for the Noise
// mesh handshake). The X25519 keys are deterministic from the
// Ed25519 seed via HKDF-SHA256, so an Identity restored from disk
// reproduces the same X25519 static key every time. PrivateKey +
// X25519Private must never leave the process they were generated in.
type Identity struct {
	NodeID     string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey

	// X25519 static keypair for Noise. 32 bytes each. Derived from
	// PrivateKey.Seed() via HKDF so we don't need a separate disk
	// artifact — restoring the Ed25519 seed reconstructs them.
	X25519Private [32]byte
	X25519Public  [32]byte
}

// x25519HKDFLabel domain-separates the X25519 derivation so the same
// Ed25519 seed cannot be accidentally reused to derive a key that
// chains into another protocol (e.g. file encryption). Kept stable
// across versions; if it ever needs to change, bump the suffix.
const x25519HKDFLabel = "platypus-mesh-noise-x25519-v1"

// deriveX25519Keypair derives a Curve25519 static keypair from an
// Ed25519 seed. The derivation is deterministic so the same seed
// reproduces the same X25519 keypair — which means persisting just
// the Ed25519 seed is enough to restore the full Identity.
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
// seed. Called by every constructor path (NewIdentity +
// LoadOrCreateIdentity) so Identity is always complete.
func (id *Identity) fillX25519() error {
	priv, pub, err := deriveX25519Keypair(id.PrivateKey.Seed())
	if err != nil {
		return fmt.Errorf("mesh: derive x25519: %w", err)
	}
	id.X25519Private = priv
	id.X25519Public = pub
	return nil
}

// DeriveNodeID returns the canonical NodeID for an Ed25519 public key.
// The mapping is pure: two callers with the same pubkey always get the
// same NodeID, and no other pubkey can produce it without breaking
// SHA-256 preimage resistance.
func DeriveNodeID(pub ed25519.PublicKey) string {
	if len(pub) != ed25519.PublicKeySize {
		return ""
	}
	sum := sha256.Sum256(pub)
	return strings.ToLower(nodeIDEncoding.EncodeToString(sum[:]))[:NodeIDLen]
}

// NewIdentity generates a fresh Ed25519 keypair and derives the
// matching X25519 static for Noise.
func NewIdentity() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("mesh: generate identity key: %w", err)
	}
	id := &Identity{
		NodeID:     DeriveNodeID(pub),
		PublicKey:  pub,
		PrivateKey: priv,
	}
	if err := id.fillX25519(); err != nil {
		return nil, err
	}
	return id, nil
}

// LoadOrCreateIdentity reads an identity from dir, or generates and
// persists a new one if no existing identity is found. Files written:
//
//	<dir>/identity.key   — PKCS#8-equivalent raw Ed25519 seed (32 bytes)
//	<dir>/identity.pub   — raw 32-byte Ed25519 public key
//
// Both are written with mode 0600. The directory is created with 0700
// if it doesn't exist.
func LoadOrCreateIdentity(dir string) (*Identity, error) {
	if dir == "" {
		return nil, errors.New("mesh: identity dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mesh: create identity dir: %w", err)
	}
	keyPath := filepath.Join(dir, "identity.key")
	pubPath := filepath.Join(dir, "identity.pub")

	seed, err := os.ReadFile(keyPath)
	if err == nil {
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("mesh: identity key at %s is %d bytes, want %d", keyPath, len(seed), ed25519.SeedSize)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		id := &Identity{
			NodeID:     DeriveNodeID(pub),
			PublicKey:  pub,
			PrivateKey: priv,
		}
		if err := id.fillX25519(); err != nil {
			return nil, err
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("mesh: read identity key: %w", err)
	}

	id, err := NewIdentity()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, id.PrivateKey.Seed(), 0o600); err != nil {
		return nil, fmt.Errorf("mesh: write identity key: %w", err)
	}
	if err := os.WriteFile(pubPath, id.PublicKey, 0o600); err != nil {
		return nil, fmt.Errorf("mesh: write identity pub: %w", err)
	}
	return id, nil
}

// LoadOrCreatePSK reads a pre-shared key from file, or generates a new
// random 32-byte PSK if the file doesn't exist. Useful for
// single-machine smoke tests; real deployments should distribute the PSK
// out-of-band and every node should point at the same bytes.
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

// decodePSK accepts either raw bytes or a base32/base64-ish textual form
// (we try base32 first because that's what LoadOrCreatePSK emits; fall
// back to the raw bytes if decoding fails). Whitespace is stripped.
func decodePSK(raw []byte) []byte {
	trimmed := strings.TrimSpace(string(raw))
	if decoded, err := base32.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) >= 16 {
		return decoded
	}
	// Accept raw bytes verbatim (e.g. operator piped random bytes).
	return []byte(trimmed)
}
