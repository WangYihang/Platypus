// Package signing provides Ed25519 detached-signature helpers used to
// authenticate the agent release manifest.
//
// The release pipeline signs the manifest bytes with a private key held
// only in CI secret storage; agents embed the corresponding public key at
// build time via -ldflags and verify it before trusting any artifact
// hash. The Distributor itself never needs the private key at runtime.
package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
)

// Sign produces a detached Ed25519 signature over data. Used by the
// release tooling, not by the server at runtime.
func Sign(priv ed25519.PrivateKey, data []byte) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("signing: invalid private key length %d", len(priv))
	}
	return ed25519.Sign(priv, data), nil
}

// Verify checks a detached Ed25519 signature.
func Verify(pub ed25519.PublicKey, data, sig []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("signing: invalid public key length %d", len(pub))
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signing: invalid signature length %d", len(sig))
	}
	if !ed25519.Verify(pub, data, sig) {
		return errors.New("signing: signature verification failed")
	}
	return nil
}

// DecodePublicKey parses a base64-encoded Ed25519 public key. The agent
// stores its trust anchor as a base64 string injected at build time, so
// this is the canonical entry point.
func DecodePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("signing: decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("signing: public key length %d != %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
