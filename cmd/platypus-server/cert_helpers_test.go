package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

// TestCertHostsForSAN locks in the SAN list shape — the auto-issued
// leaf must always cover the configured external host plus the
// loopback names (so curl https://localhost:9443/... and curl
// https://127.0.0.1:9443/... verify against the same cert without the
// operator lining hostnames up with public_addr).
func TestCertHostsForSAN(t *testing.T) {
	tests := []struct {
		name         string
		externalAddr string
		want         []string
	}{
		{
			name:         "host:port",
			externalAddr: "platypus.example.com:9443",
			want:         []string{"platypus.example.com", "localhost", "127.0.0.1", "::1"},
		},
		{
			name:         "host without port",
			externalAddr: "platypus.example.com",
			want:         []string{"platypus.example.com", "localhost", "127.0.0.1", "::1"},
		},
		{
			name:         "external is loopback already — no duplicates",
			externalAddr: "localhost:9443",
			want:         []string{"localhost", "127.0.0.1", "::1"},
		},
		{
			name:         "empty external",
			externalAddr: "",
			want:         nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Options{ExternalAddr: tc.externalAddr}
			got := certHostsForSAN(cfg)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d]=%q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCertCoversHosts verifies the on-disk SAN check that gates the
// "external-addr changed since last run, re-issue" path. False
// positives keep stale certs in service; false negatives churn a
// valid cert. Both are bad.
func TestCertCoversHosts(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")

	writeCertWithSAN(t, certPath, []string{"platypus.example.com", "localhost"}, []string{"127.0.0.1", "::1"})

	tests := []struct {
		name  string
		wants []string
		want  bool
	}{
		{name: "exact match", wants: []string{"platypus.example.com"}, want: true},
		{name: "loopback hosts covered", wants: []string{"localhost", "127.0.0.1"}, want: true},
		{name: "ipv6 loopback covered", wants: []string{"::1"}, want: true},
		{name: "case-insensitive DNS match", wants: []string{"PLATYPUS.example.COM"}, want: true},
		{name: "missing host triggers re-issue", wants: []string{"new-host.example.com"}, want: false},
		{name: "empty wants is trivially covered", wants: nil, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := certCoversHosts(certPath, tc.wants); got != tc.want {
				t.Errorf("certCoversHosts(%v) = %v, want %v", tc.wants, got, tc.want)
			}
		})
	}

	t.Run("missing file → false (re-issue)", func(t *testing.T) {
		if certCoversHosts(filepath.Join(dir, "nope.pem"), []string{"x"}) {
			t.Error("missing cert file should not satisfy coverage")
		}
	})

	t.Run("garbage PEM → false", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.pem")
		if err := os.WriteFile(bad, []byte("not a pem"), 0o600); err != nil {
			t.Fatal(err)
		}
		if certCoversHosts(bad, []string{"x"}) {
			t.Error("unparseable cert should not satisfy coverage")
		}
	})
}

// TestPersistAutoIssuedLeaf covers the file-mode + half-pair contract:
// we never want a half-written cert without its key (or vice versa)
// because the next startup would mistake an orphan cert for a complete
// auto-managed pair and skip re-issue.
func TestPersistAutoIssuedLeaf(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	persistAutoIssuedLeaf(certPath, keyPath, "CERT", "KEY")

	if got, _ := os.ReadFile(certPath); string(got) != "CERT" {
		t.Errorf("cert content = %q, want %q", got, "CERT")
	}
	if got, _ := os.ReadFile(keyPath); string(got) != "KEY" {
		t.Errorf("key content = %q, want %q", got, "KEY")
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode := keyInfo.Mode().Perm(); mode != 0o600 {
		t.Errorf("key mode = %#o, want 0600", mode)
	}
}

// writeCertWithSAN mints a tiny self-signed cert with the requested
// DNS / IP SANs and writes it as PEM at the given path. ECDSA P-256
// keeps the test fast.
func writeCertWithSAN(t *testing.T, path string, dnsNames, ipStrings []string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     dnsNames,
	}
	for _, s := range ipStrings {
		if ip := net.ParseIP(s); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}
