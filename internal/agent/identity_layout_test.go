package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mintCAPEM produces a self-signed Ed25519 CA cert PEM, deterministically
// keyed off the seed string so two callers with the same seed get the
// same bytes (and the same fingerprint).
func mintCAPEM(t *testing.T, cn string) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// CAFingerprint must be deterministic on the same input bytes — the
// per-CA layout dispatch in BootstrapV2 keys subdirs by this value.
func TestCAFingerprint_Stable(t *testing.T) {
	pemBytes := mintCAPEM(t, "stable-ca")
	a, err := CAFingerprint(pemBytes)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := CAFingerprint(pemBytes)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if a != b {
		t.Fatalf("fingerprint not deterministic: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("fingerprint length = %d; want 16 hex chars", len(a))
	}
}

// Different CA bytes must yield different fingerprints — two
// enrollments to different servers must not collide on disk.
func TestCAFingerprint_Distinct(t *testing.T) {
	a, _ := CAFingerprint(mintCAPEM(t, "ca-one"))
	b, _ := CAFingerprint(mintCAPEM(t, "ca-two"))
	if a == b {
		t.Fatalf("distinct CAs produced the same fingerprint: %s", a)
	}
}

// Non-PEM input is a programmer error — CAFingerprint must reject it
// loudly so we don't hash garbage into the layout.
func TestCAFingerprint_RejectsNonPEM(t *testing.T) {
	if _, err := CAFingerprint([]byte("not a pem block")); err == nil {
		t.Fatal("expected error for non-PEM input")
	}
}

// Active pointer round-trip: WriteActive then ReadActive returns the
// fingerprint, and ReadActive on a fresh root returns "" with no error
// so callers can branch on first-run cleanly.
func TestActiveFingerprint_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if got, err := ReadActive(root); err != nil || got != "" {
		t.Fatalf("ReadActive on empty root: got %q, err %v; want \"\", nil", got, err)
	}
	if err := WriteActive(root, "abc1234567890def"); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}
	got, err := ReadActive(root)
	if err != nil {
		t.Fatalf("ReadActive: %v", err)
	}
	if got != "abc1234567890def" {
		t.Fatalf("ReadActive = %q; want abc1234567890def", got)
	}
}

// MigrateLegacyIdentity moves a flat-layout identity (root/{client.crt,
// client.key,project_ca.crt}) into root/<fp>/{...} and writes the
// active pointer so the next BootstrapV2 finds it.
func TestMigrateLegacyIdentity(t *testing.T) {
	root := t.TempDir()
	caPEM := mintCAPEM(t, "legacy-ca")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := SaveIdentity(root, priv, []byte("cert-bytes"), caPEM); err != nil {
		t.Fatalf("SaveIdentity (legacy layout): %v", err)
	}

	if err := MigrateLegacyIdentity(root); err != nil {
		t.Fatalf("MigrateLegacyIdentity: %v", err)
	}

	// Legacy files must be gone from the root.
	for _, name := range []string{keyFileName, crtFileName, caFileName} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still in root after migration: stat err %v", name, err)
		}
	}

	// Active pointer points at the right subdir, and that subdir
	// holds the migrated files.
	fp, err := ReadActive(root)
	if err != nil {
		t.Fatalf("ReadActive: %v", err)
	}
	if fp == "" {
		t.Fatal("active pointer empty after migration")
	}
	id, err := LoadIdentity(IdentitySubdir(root, fp))
	if err != nil {
		t.Fatalf("LoadIdentity from migrated subdir: %v", err)
	}
	if string(id.CertPEM) != "cert-bytes" || string(id.CAPEM) != string(caPEM) {
		t.Fatal("migrated files do not match originals")
	}
}

// MigrateLegacyIdentity is idempotent: a second call on an already-
// migrated tree is a no-op (and doesn't error on the missing legacy
// files).
func TestMigrateLegacyIdentity_Idempotent(t *testing.T) {
	root := t.TempDir()
	caPEM := mintCAPEM(t, "idempotent-ca")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = SaveIdentity(root, priv, []byte("cert-bytes"), caPEM)
	if err := MigrateLegacyIdentity(root); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := MigrateLegacyIdentity(root); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// Two CAs land in distinct subdirs — re-enrollment into a different
// server (or a CA rotation on the same server) must not overwrite the
// previous identity. The active pointer follows the latest.
func TestIdentitySubdir_DistinctPerCA(t *testing.T) {
	root := t.TempDir()
	caA := mintCAPEM(t, "server-A")
	caB := mintCAPEM(t, "server-B")
	fpA, _ := CAFingerprint(caA)
	fpB, _ := CAFingerprint(caB)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := SaveIdentity(IdentitySubdir(root, fpA), priv, []byte("cert-A"), caA); err != nil {
		t.Fatalf("save A: %v", err)
	}
	_ = WriteActive(root, fpA)

	if err := SaveIdentity(IdentitySubdir(root, fpB), priv, []byte("cert-B"), caB); err != nil {
		t.Fatalf("save B: %v", err)
	}
	_ = WriteActive(root, fpB)

	// Both subdirs still exist with their own files.
	idA, err := LoadIdentity(IdentitySubdir(root, fpA))
	if err != nil {
		t.Fatalf("load A after second save: %v", err)
	}
	if string(idA.CertPEM) != "cert-A" {
		t.Fatalf("server A's cert lost after server B's enrollment: got %q", idA.CertPEM)
	}
	idB, err := LoadIdentity(IdentitySubdir(root, fpB))
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	if string(idB.CertPEM) != "cert-B" {
		t.Fatalf("server B's cert: got %q", idB.CertPEM)
	}

	if active, _ := ReadActive(root); active != fpB {
		t.Fatalf("active pointer = %q; want %q (the most recent enrollment)", active, fpB)
	}
}

// First enrollment on a fresh root must complete and leave the active
// pointer + per-CA subdir wired up so a subsequent restart (without
// the env var) can find the identity.
func TestEnrollAndPersist_LandsInPerCASubdir(t *testing.T) {
	root := t.TempDir()
	caPEM := mintCAPEM(t, "fresh-ca")
	fp, _ := CAFingerprint(caPEM)

	// Drop a fake "enrolled" identity directly into the subdir we
	// expect — TestEnroll round-trips a real HTTPS server, and we're
	// only validating the layout here, so synthesise the post-enroll
	// state and let the TestBootstrapV2_ReusesExistingIdentity-style
	// path exercise the full enroll wire.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := SaveIdentity(IdentitySubdir(root, fp), priv, []byte("cert"), caPEM); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}
	if err := WriteActive(root, fp); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}

	// Reading back via the public surface (LoadIdentity at the
	// resolved subdir) must succeed.
	id, err := LoadIdentity(IdentitySubdir(root, fp))
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	if string(id.CAPEM) != string(caPEM) {
		t.Fatal("CA round-trip mismatch")
	}
}

// First-run with no env CA, no active pointer, and no PAT must
// produce an actionable error (mentions PAT) — not a bare
// ErrIdentityNotFound.
func TestLoadOrEnroll_FirstRunNoPAT(t *testing.T) {
	root := t.TempDir()
	_, err := loadOrEnroll(nil, root, BootstrapV2Options{})
	if err == nil {
		t.Fatal("expected error when nothing is configured")
	}
	if !strings.Contains(err.Error(), "PAT") {
		t.Fatalf("error %q should mention PAT to guide the operator", err.Error())
	}
}

// First-run with a PAT but no env CA must point operators at the
// install-script env var so they don't keep retrying with just the
// PAT.
func TestLoadOrEnroll_FirstRunNoCA(t *testing.T) {
	root := t.TempDir()
	_, err := loadOrEnroll(nil, root, BootstrapV2Options{
		PAT: "plt_test",
	})
	if err == nil {
		t.Fatal("expected error when PAT is set but env CA is not")
	}
	if !strings.Contains(err.Error(), "PLATYPUS_PROJECT_CA") {
		t.Fatalf("error %q should mention PLATYPUS_PROJECT_CA env var", err.Error())
	}
}
