package mesh

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/utils/crypto"
)

// selfSignedTLSConfig builds an outbound TLS config using the shared
// self-signed cert helper. Mutual identity is proven at the
// application layer via the mesh handshake (PSK + Ed25519), so the
// InsecureSkipVerify on cert-chain level is acceptable here. ALPN
// announces "ptps-mesh" so the server's unified-ingress dispatcher
// routes these connections to Node.AcceptRaw.
func selfSignedTLSConfig() (*tls.Config, error) {
	certBuilder := &strings.Builder{}
	keyBuilder := &strings.Builder{}
	crypto.Generate(certBuilder, keyBuilder)
	cert, err := tls.X509KeyPair([]byte(certBuilder.String()), []byte(keyBuilder.String()))
	if err != nil {
		return nil, fmt.Errorf("mesh tls config: %w", err)
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"ptps-mesh"},
	}, nil
}
