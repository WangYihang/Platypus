package agent

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// ALPNAgentV2 is the ALPN protocol identifier the agent advertises
// to the server's ingress dispatcher so agent-link traffic lands on
// the v2 handler (distinct from the legacy "ptps-agent" Envelope
// flow, which Phase III will retire). For now we reuse the same
// string — the v1 Envelope pipeline is still the dispatcher's agent
// handler, and once Phase II.6 lands the server-side handler
// switches to WS/yamux.
const ALPNAgentV2 = "ptps-agent"

// BuildDialerTLSConfig produces a *tls.Config ready for tls.Dial
// from an on-disk agent Identity. The caller uses this as the
// Transport.TLSClientConfig when dialing the server over HTTPS
// before upgrading to WebSocket.
//
// Invariants:
//   - Certificates[0] is the agent's own leaf + private key.
//   - RootCAs is a pool containing only the project CA.
//   - NextProtos advertises ALPNAgentV2.
//   - MinVersion = TLS 1.2.
//   - InsecureSkipVerify is hard-coded false; callers that want
//     to skip verification should do so at a higher layer during
//     first-time enrollment, not through this helper.
func BuildDialerTLSConfig(id *Identity) (*tls.Config, error) {
	if id == nil {
		return nil, errors.New("agent: BuildDialerTLSConfig: nil Identity")
	}

	keyPEM, err := marshalPrivateKeyPEM(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("agent: BuildDialerTLSConfig marshal key: %w", err)
	}
	cert, err := tls.X509KeyPair(id.CertPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: BuildDialerTLSConfig parse client cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(id.CAPEM) {
		return nil, errors.New("agent: BuildDialerTLSConfig: project CA PEM parse failed")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		NextProtos:   []string{ALPNAgentV2},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// marshalPrivateKeyPEM wraps an Ed25519 private key in a PKCS#8 PEM
// block so tls.X509KeyPair can consume it. Identical on-disk and
// in-memory format: SaveIdentity writes exactly this shape.
func marshalPrivateKeyPEM(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
