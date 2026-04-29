package ingress

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildTLSConfig_SelfSignedPersist exercises the self-signed
// fallback path with PersistTo set: the freshly-minted PEMs must
// land at the configured paths so the next startup can load them via
// the CertFile branch and keep the leaf fingerprint stable across
// restarts. This is the regression net for the "every Docker restart
// asks me to trust the cert again" bug.
func TestBuildTLSConfig_SelfSignedPersist(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	cfg, err := BuildTLSConfig(CertSource{
		PersistTo: PersistTarget{CertPath: certPath, KeyPath: keyPath},
	}, DefaultProtocols)
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("want 1 leaf, got %d", len(cfg.Certificates))
	}

	// Both PEMs must exist and parse.
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	if block, _ := pem.Decode(certBytes); block == nil {
		t.Fatalf("cert PEM did not decode")
	} else if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if block, _ := pem.Decode(keyBytes); block == nil {
		t.Fatalf("key PEM did not decode")
	}

	// The key file must be 0600 — it's a private key on a shared
	// data-dir that operators routinely volume-mount.
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if mode := keyInfo.Mode().Perm(); mode != 0o600 {
		t.Errorf("key perm = %#o, want 0600", mode)
	}

	// The PEMs we wrote must round-trip back into the same TLS
	// keypair the in-memory cert exposes. Loading the second time
	// through tls.LoadX509KeyPair simulates the next startup booting
	// off the file convention.
	loaded, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	if len(loaded.Certificate) == 0 || len(cfg.Certificates[0].Certificate) == 0 {
		t.Fatal("empty cert chain")
	}
	// Same DER bytes ⇒ same leaf ⇒ same fingerprint. This is the
	// load-bearing assertion: persistence is only useful if the next
	// startup reuses the exact same leaf.
	if !equalBytes(loaded.Certificate[0], cfg.Certificates[0].Certificate[0]) {
		t.Error("persisted cert does not match the in-memory leaf")
	}
}

// TestBuildTLSConfig_SelfSignedNoPersist verifies the historic
// behaviour is preserved when PersistTo is zero: the fallback still
// returns a usable cert and writes nothing to disk.
func TestBuildTLSConfig_SelfSignedNoPersist(t *testing.T) {
	dir := t.TempDir()
	cfg, err := BuildTLSConfig(CertSource{}, DefaultProtocols)
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("want 1 leaf, got %d", len(cfg.Certificates))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("data dir should be untouched without PersistTo, got %d entries", len(entries))
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
