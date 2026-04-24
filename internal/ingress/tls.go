package ingress

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
)

// CertSource describes where to get the server's TLS certificate.
// Either LoadFromFiles to point at an operator-provided cert/key pair,
// or SelfSigned to generate an ephemeral one at startup.
type CertSource struct {
	// CertFile / KeyFile point at PEM-encoded cert + key. Both must be
	// set together; if either is empty the source falls back to
	// self-signed and logs a loud warning.
	CertFile string
	KeyFile  string
}

// BuildTLSConfig returns a *tls.Config wired up for ALPN dispatch. On
// a self-signed fallback it logs a prominent warning — admins who see
// this in production need to provision a real certificate because
// browsers, agents, and the curl | sh bootstrap all hit the same port
// now.
func BuildTLSConfig(src CertSource, protocols []string) (*tls.Config, error) {
	if len(protocols) == 0 {
		protocols = DefaultProtocols
	}

	cert, err := loadOrGenerate(src)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   protocols,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func loadOrGenerate(src CertSource) (tls.Certificate, error) {
	if src.CertFile != "" && src.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(src.CertFile, src.KeyFile)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("load cert %q / key %q: %w",
				src.CertFile, src.KeyFile, err)
		}
		log.L.Info("ingress_tls_cert_loaded",
			"cert_file", src.CertFile,
			"key_file", src.KeyFile,
		)
		return cert, nil
	}

	// Fallback: the same self-signed generator the pre-unification
	// code used. Fine for dev and first-boot scenarios; a persistent
	// deployment should provision a real cert.
	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)
	crypto.Generate(certBuilder, keyBuilder)

	cert, err := tls.X509KeyPair([]byte(certBuilder.String()), []byte(keyBuilder.String()))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("self-signed cert: %w", err)
	}

	log.L.Warn("ingress_tls_cert_self_signed",
		"hint", "browsers will warn and curl needs -k. set Server.TLSCert and Server.TLSKey in production.",
		"pid", os.Getpid(),
	)
	return cert, nil
}
