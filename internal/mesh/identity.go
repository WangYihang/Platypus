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
	"os"
	"path/filepath"
	"strings"
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

// Identity bundles an Ed25519 keypair with its derived NodeID. It is
// safe to share the exported PublicKey / NodeID; PrivateKey must never
// leave the process it was generated in.
type Identity struct {
	NodeID     string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
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

// NewIdentity generates a fresh Ed25519 keypair.
func NewIdentity() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("mesh: generate identity key: %w", err)
	}
	return &Identity{
		NodeID:     DeriveNodeID(pub),
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
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
		return &Identity{
			NodeID:     DeriveNodeID(pub),
			PublicKey:  pub,
			PrivateKey: priv,
		}, nil
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
