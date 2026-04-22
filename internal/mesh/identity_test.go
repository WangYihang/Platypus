package mesh

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveNodeIDStable(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	a := DeriveNodeID(pub)
	b := DeriveNodeID(pub)
	if a != b {
		t.Fatalf("DeriveNodeID not deterministic: %q vs %q", a, b)
	}
	if len(a) != NodeIDLen {
		t.Fatalf("NodeID length = %d, want %d", len(a), NodeIDLen)
	}
}

func TestDeriveNodeIDRejectsWrongLen(t *testing.T) {
	if DeriveNodeID([]byte{1, 2, 3}) != "" {
		t.Fatal("expected empty NodeID for invalid pubkey")
	}
}

func TestLoadOrCreateIdentityPersists(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if first.NodeID != second.NodeID {
		t.Fatalf("NodeID changed between loads: %q vs %q", first.NodeID, second.NodeID)
	}
	if !bytes.Equal(first.PublicKey, second.PublicKey) {
		t.Fatal("pubkey not stable across loads")
	}
	// Check file modes — identity key must not be world-readable.
	info, err := os.Stat(filepath.Join(dir, "identity.key"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity.key perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadOrCreatePSKRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "psk.bin")
	psk1, err := LoadOrCreatePSK(path)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	psk2, err := LoadOrCreatePSK(path)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(psk1, psk2) {
		t.Fatal("PSK not stable across loads")
	}
	if len(psk1) < 16 {
		t.Fatalf("PSK too short: %d bytes", len(psk1))
	}
}
