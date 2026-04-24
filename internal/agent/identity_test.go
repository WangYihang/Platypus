package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// SaveIdentity writes the agent's three identity files (private key,
// client cert, project CA) to the supplied directory. The pair is
// what the dialer will load on every reconnect, so the on-disk
// format must round-trip losslessly.

// Round-trip: Save then Load returns byte-identical material, and
// the reconstituted ed25519 PrivateKey matches the original.
func TestSaveLoadIdentity_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	certPEM := []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n")
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nBBBB\n-----END CERTIFICATE-----\n")

	if err := SaveIdentity(dir, priv, certPEM, caPEM); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	id, err := LoadIdentity(dir)
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	if !id.PrivateKey.Equal(priv) {
		t.Fatal("loaded private key differs from saved key")
	}
	if !bytes.Equal(id.CertPEM, certPEM) {
		t.Fatalf("CertPEM round-trip mismatch: got %q; want %q", id.CertPEM, certPEM)
	}
	if !bytes.Equal(id.CAPEM, caPEM) {
		t.Fatalf("CAPEM round-trip mismatch: got %q; want %q", id.CAPEM, caPEM)
	}
}

// Any of the three files missing → ErrIdentityNotFound. The caller
// branches on that to trigger enrollment instead of aborting.
func TestLoadIdentity_MissingDir(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := LoadIdentity(nonExistent); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("err = %v; want ErrIdentityNotFound", err)
	}
}

func TestLoadIdentity_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// Only write the cert; key and CA are missing.
	if err := os.WriteFile(filepath.Join(dir, "client.crt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("prep: %v", err)
	}
	if _, err := LoadIdentity(dir); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("err = %v; want ErrIdentityNotFound", err)
	}
}

// Persisted files must be mode 0600: the private key in particular
// is a long-lived credential, and a world-readable ~/.platypus/
// would be a disclosure bug.
func TestSaveIdentity_FilesMode0600(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := SaveIdentity(dir, priv, []byte("cert"), []byte("ca")); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}
	for _, name := range []string{"client.key", "client.crt", "project_ca.crt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o; want 0600", name, info.Mode().Perm())
		}
	}
}

// SaveIdentity must create the directory if missing, with restrictive
// perms (0700). Agents starting from scratch shouldn't have to
// mkdir the identity dir themselves.
func TestSaveIdentity_CreatesDir(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "deeply", "nested", "id")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := SaveIdentity(nested, priv, []byte("cert"), []byte("ca")); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o; want 0700", info.Mode().Perm())
	}
}
