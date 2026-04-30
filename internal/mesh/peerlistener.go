package mesh

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// PeerListener exposes /api/v1/mesh/link on an agent process so other
// nodes can dial in directly. Identity (cert + key + project CA pool)
// matches what the agent uses to dial out — the same leaf cert holds
// both serverAuth and clientAuth EKUs.
type PeerListener struct {
	srv      *http.Server
	listener net.Listener
}

// NewPeerListener binds addr and prepares the mTLS+HTTP/2 listener.
// Caller invokes Serve in a goroutine and Shutdown on teardown.
func NewPeerListener(addr string, id *Identity, caPool *x509.CertPool, handler http.Handler) (*PeerListener, error) {
	if id == nil || len(id.CertPEM) == 0 || id.PrivateKey == nil {
		return nil, errors.New("mesh: PeerListener identity must carry CertPEM + PrivateKey")
	}
	if caPool == nil {
		return nil, errors.New("mesh: PeerListener requires project CA pool")
	}
	keyPEM, err := marshalEd25519PKCS8(id.PrivateKey)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(id.CertPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("mesh: PeerListener X509KeyPair: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}
	mux := http.NewServeMux()
	mux.Handle(LinkPath, handler)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
		return nil, fmt.Errorf("mesh: http2.ConfigureServer: %w", err)
	}
	rawLn, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mesh: PeerListener listen %s: %w", addr, err)
	}
	tlsLn := tls.NewListener(rawLn, tlsCfg)
	return &PeerListener{srv: srv, listener: tlsLn}, nil
}

func (p *PeerListener) Addr() string { return p.listener.Addr().String() }

func (p *PeerListener) Serve() error {
	if err := p.srv.Serve(p.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (p *PeerListener) Shutdown(ctx context.Context) error {
	return p.srv.Shutdown(ctx)
}
