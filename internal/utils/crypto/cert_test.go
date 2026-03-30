package crypto

import (
	"strings"
	"testing"
)

func TestGenerateCert(t *testing.T) {
	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)

	Generate(certBuilder, keyBuilder)

	cert := certBuilder.String()
	key := keyBuilder.String()

	if cert == "" {
		t.Fatal("cert should not be empty")
	}
	if key == "" {
		t.Fatal("key should not be empty")
	}

	if !strings.Contains(cert, "BEGIN CERTIFICATE") {
		t.Error("cert should contain PEM certificate header")
	}
	if !strings.Contains(key, "BEGIN") {
		t.Error("key should contain PEM key header")
	}
}

func TestGenerateCertUniqueness(t *testing.T) {
	cert1 := new(strings.Builder)
	key1 := new(strings.Builder)
	Generate(cert1, key1)

	cert2 := new(strings.Builder)
	key2 := new(strings.Builder)
	Generate(cert2, key2)

	if cert1.String() == cert2.String() {
		t.Error("two generated certs should be different")
	}
}
