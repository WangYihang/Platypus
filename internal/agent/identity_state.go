package agent

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// activeFile holds the CA fingerprint of the currently-active enrollment
// under the identity root. Restarts that don't carry PLATYPUS_PROJECT_CA
// (the install script only sets it on the first run) read this file to
// find the per-CA subdirectory that owns the live identity + mesh state.
const activeFile = "active"

// ResolveIdentityDir returns the effective persistent state ROOT — the
// parent under which per-CA identity subdirectories live (one per
// enrollment, scoped by IdentitySubdir / CAFingerprint). The root itself
// is not where cert/key files land directly; that used to be the layout
// but pre-dates the multi-CA support.
func ResolveIdentityDir(identityDir string) string {
	if identityDir != "" {
		return identityDir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".platypus-agent"
	}
	return filepath.Join(home, ".platypus", "agent")
}

// CAFingerprint returns a stable, filesystem-friendly identifier for a
// project CA. The first 16 hex chars (8 bytes / 64 bits) of the SHA-256
// of the CA's DER bytes — short enough to read in `ls`, wide enough that
// random Ed25519 CAs won't collide in any realistic deployment.
//
// Used as the per-CA subdirectory name under the identity root so an
// agent that re-enrolls into a new server (or sees its old server's CA
// rotate) doesn't overwrite identity material that still belongs to a
// different enrollment.
func CAFingerprint(caPEM []byte) (string, error) {
	block, _ := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("agent: CAFingerprint: input is not a CERTIFICATE PEM block")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", fmt.Errorf("agent: CAFingerprint: parse cert: %w", err)
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:8]), nil
}

// IdentitySubdir returns the per-CA subdirectory under root that holds
// one enrollment's identity files (cert/key/CA + mesh state). Multiple
// fingerprints coexist under the same root.
func IdentitySubdir(root, fingerprint string) string {
	return filepath.Join(root, fingerprint)
}

// activePath is the small text file at the identity root whose content
// is the CA fingerprint of the currently-active enrollment.
func activePath(root string) string {
	return filepath.Join(root, activeFile)
}

// WriteActive atomically updates the active-fingerprint pointer.
// MkdirAll keeps callers from having to pre-create the root.
func WriteActive(root, fingerprint string) error {
	if fingerprint == "" {
		return errors.New("agent: WriteActive: fingerprint is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("agent: WriteActive mkdir %s: %w", root, err)
	}
	tmp := activePath(root) + ".tmp"
	if err := os.WriteFile(tmp, []byte(fingerprint+"\n"), 0o600); err != nil {
		return fmt.Errorf("agent: WriteActive write: %w", err)
	}
	return os.Rename(tmp, activePath(root))
}

// HasPersistedIdentity reports whether the agent has a usable
// identity already on disk under root. Cheap stat-only check used by
// main.go to decide whether the user needs to supply an install token
// and server endpoint on the command line, or whether a re-run can
// reuse the cached cert + a server endpoint baked into the active
// pointer's metadata.
func HasPersistedIdentity(root string) bool {
	fp, err := ReadActive(root)
	if err != nil || fp == "" {
		return false
	}
	if _, err := LoadIdentity(root); err == nil {
		return true
	}
	return false
}

// ReadActive returns the active fingerprint stored under root, or
// the empty string when no pointer has been written yet (so first-
// boot callers can branch on that instead of an error).
func ReadActive(root string) (string, error) {
	b, err := os.ReadFile(activePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("agent: ReadActive: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// MigrateLegacyIdentity moves a flat-layout identity (root/{client.crt,
// client.key,project_ca.crt}) into root/<fp>/{...} on first run with
// the new code, and writes the active pointer. Idempotent — returns
// nil with no work done when the legacy layout isn't present (already
// migrated, or never enrolled).
func MigrateLegacyIdentity(root string) error {
	legacyCrt := filepath.Join(root, crtFileName)
	if _, err := os.Stat(legacyCrt); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	caPath := filepath.Join(root, caFileName)
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("agent: migrate legacy identity: read CA: %w", err)
	}
	fp, err := CAFingerprint(caPEM)
	if err != nil {
		return fmt.Errorf("agent: migrate legacy identity: %w", err)
	}
	sub := IdentitySubdir(root, fp)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		return fmt.Errorf("agent: migrate legacy identity: mkdir %s: %w", sub, err)
	}
	for _, name := range []string{keyFileName, crtFileName, caFileName} {
		src := filepath.Join(root, name)
		dst := filepath.Join(sub, name)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("agent: migrate legacy identity: move %s: %w", name, err)
		}
	}
	return WriteActive(root, fp)
}
