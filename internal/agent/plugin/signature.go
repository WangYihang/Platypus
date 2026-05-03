package plugin

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/jedisct1/go-minisign"
)

// HumanKeyID returns a stable, operator-readable identifier for a
// minisign public key. The minisign format reserves an 8-byte KeyId
// inside the key material; we render it as 16 hex chars (the same
// shape minisign(1) prints in its `untrusted comment`). Used both as
// the publishers/<keyid>.pub filename and in audit log records so
// rotating one key never collides with another in display.
func HumanKeyID(pk minisign.PublicKey) string {
	return hex.EncodeToString(pk.KeyId[:])
}

// LoadPublicKey parses a minisign public key file (the one-line file
// produced by `minisign -G`). Returns the parsed key plus its human
// id so callers don't have to call HumanKeyID separately.
func LoadPublicKey(path string) (minisign.PublicKey, string, error) {
	pk, err := minisign.NewPublicKeyFromFile(path)
	if err != nil {
		return minisign.PublicKey{}, "", fmt.Errorf("plugin: load pubkey %s: %w", path, err)
	}
	return pk, HumanKeyID(pk), nil
}

// LoadPublicKeyFromBytes parses an in-memory minisign public key. Used
// when the server pushes a publisher pubkey alongside an install
// request rather than relying on a pre-seeded publishers/ file.
func LoadPublicKeyFromBytes(b []byte) (minisign.PublicKey, string, error) {
	pk, err := minisign.DecodePublicKey(string(b))
	if err != nil {
		return minisign.PublicKey{}, "", fmt.Errorf("plugin: decode pubkey: %w", err)
	}
	return pk, HumanKeyID(pk), nil
}

// LoadSignature parses a detached .minisig file.
func LoadSignature(path string) (minisign.Signature, error) {
	sig, err := minisign.NewSignatureFromFile(path)
	if err != nil {
		return minisign.Signature{}, fmt.Errorf("plugin: load sig %s: %w", path, err)
	}
	return sig, nil
}

// LoadSignatureFromBytes parses an in-memory detached .minisig.
func LoadSignatureFromBytes(b []byte) (minisign.Signature, error) {
	sig, err := minisign.DecodeSignature(string(b))
	if err != nil {
		return minisign.Signature{}, fmt.Errorf("plugin: decode sig: %w", err)
	}
	return sig, nil
}

// VerifyWasm checks that `wasmBytes` was signed by `pk` and the
// detached signature in `sigPath`. Returns nil on success; a wrapped
// error otherwise. The minisign library returns a bool+error pair where
// success is (true, nil) and verification failure is (false, err); we
// flatten that here so callers don't accidentally treat (false, nil)
// as success.
func VerifyWasm(pk minisign.PublicKey, wasmBytes []byte, sig minisign.Signature) error {
	ok, err := pk.Verify(wasmBytes, sig)
	if err != nil {
		return fmt.Errorf("plugin: signature verify: %w", err)
	}
	if !ok {
		return errors.New("plugin: signature verify: did not match")
	}
	return nil
}

// VerifyWasmFile is the on-disk convenience wrapper for VerifyWasm:
// reads the .wasm and .minisig from disk before verifying. The agent
// loader uses this on every cold start of an installed plugin so a
// tampered binary is caught before instantiation.
func VerifyWasmFile(pk minisign.PublicKey, wasmPath, sigPath string) error {
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("plugin: read wasm: %w", err)
	}
	sig, err := LoadSignature(sigPath)
	if err != nil {
		return err
	}
	return VerifyWasm(pk, wasm, sig)
}
