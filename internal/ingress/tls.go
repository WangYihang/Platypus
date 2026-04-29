package ingress

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
)

// PersistTarget names the on-disk PEM paths where the self-signed
// fallback should write its freshly-generated keypair so the next
// startup can reuse it. Empty paths disable persistence (the historic
// behaviour). Used only by the self-signed fallback path —
// project-CA-issued leafs are persisted by the caller.
type PersistTarget struct {
	CertPath string
	KeyPath  string
}

// CertSource describes where to get the server's TLS certificate.
// Priority is InMemoryCert > (CertFile+KeyFile) > self-signed
// fallback; each mode logs a distinct ingress_tls_cert_* INFO/WARN
// so operators can see which path the server took.
type CertSource struct {
	// CertFile / KeyFile point at PEM-encoded cert + key. Both must be
	// set together; if either is empty the source falls back to
	// self-signed and logs a loud warning.
	CertFile string
	KeyFile  string

	// InMemoryCert, if non-nil, is used directly without reading from
	// disk. This is the path cmd/platypus-server takes when no cert
	// file is configured: it self-issues a leaf from the project CA
	// so agents pinning the same CA pass the handshake. Takes
	// precedence over CertFile / KeyFile.
	InMemoryCert *tls.Certificate

	// PersistTo, when set, asks the self-signed fallback to write its
	// freshly-minted PEMs to these paths after generation. Without
	// this the fallback would mint a brand-new keypair on every
	// startup and invalidate every browser's "trust this cert"
	// exception. Has no effect on the InMemoryCert / CertFile paths.
	PersistTo PersistTarget
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
	if src.InMemoryCert != nil {
		log.L.Info("ingress_tls_cert_in_memory",
			"source", "project-ca-signed leaf minted at startup",
		)
		return *src.InMemoryCert, nil
	}
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

	certPEM, keyPEM := certBuilder.String(), keyBuilder.String()
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("self-signed cert: %w", err)
	}

	log.L.Warn("ingress_tls_cert_self_signed",
		"hint", "browsers will warn and curl needs -k. set Server.TLSCert and Server.TLSKey in production.",
		"pid", os.Getpid(),
	)

	// Persist the freshly-minted PEMs so the next startup picks them
	// up via the CertFile branch above and the leaf fingerprint stays
	// stable across restarts. Best-effort: a read-only data dir or a
	// half-written pair is recoverable on the next boot, so we log
	// and carry on rather than aborting.
	if src.PersistTo.CertPath != "" && src.PersistTo.KeyPath != "" {
		if err := os.WriteFile(src.PersistTo.CertPath, []byte(certPEM), 0o644); err != nil {
			log.L.Warn("ingress_tls_persist_cert_failed",
				"path", src.PersistTo.CertPath, "error", err.Error())
		} else if err := os.WriteFile(src.PersistTo.KeyPath, []byte(keyPEM), 0o600); err != nil {
			log.L.Warn("ingress_tls_persist_key_failed",
				"path", src.PersistTo.KeyPath, "error", err.Error())
			_ = os.Remove(src.PersistTo.CertPath)
		} else {
			log.L.Info("ingress_tls_cert_persisted",
				"cert_path", src.PersistTo.CertPath,
				"key_path", src.PersistTo.KeyPath)
		}
	}

	return cert, nil
}
