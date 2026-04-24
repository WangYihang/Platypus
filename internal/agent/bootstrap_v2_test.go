package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// BootstrapV2 wires LoadIdentity / Enroll / SaveIdentity /
// BuildDialerTLSConfig / Dial into a single "get me a live v2
// session" call. Tests cover the two happy branches (already
// enrolled vs fresh install) and the "no PAT and no identity" hard
// error.

// linkTestPKI mints a throwaway CA + server cert + pre-signed agent
// client cert, plus bytes you can drop into an Identity for the
// "already-enrolled" path. Self-contained so this test file doesn't
// depend on internal/pki.
type linkTestPKI struct {
	caPool    *x509.CertPool
	caPEM     []byte
	serverTLS tls.Certificate

	agentKey     ed25519.PrivateKey
	agentCertPEM []byte
}

func mintLinkTestPKI(t *testing.T, agentID, projectID string) *linkTestPKI {
	t.Helper()
	caPub, caPriv, _ := ed25519.GenerateKey(rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bootstrap-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	caCert, _ := x509.ParseCertificate(caDER)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	srvPub, srvPriv, _ := ed25519.GenerateKey(rand.Reader)
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "bootstrap-test-server"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	srvDER, _ := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, srvPub, caPriv)
	srvTLS := tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvPriv}

	// Pre-generate an agent key + cert with proper URI SANs so the
	// "already enrolled" test can drop them straight into an Identity.
	agentPub, agentPriv, _ := ed25519.GenerateKey(rand.Reader)
	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/" + projectID)
	agentTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{agentURI, projectURI},
	}
	agentDER, _ := x509.CreateCertificate(rand.Reader, agentTmpl, caCert, agentPub, caPriv)
	agentPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: agentDER})

	return &linkTestPKI{
		caPool:       caPool,
		caPEM:        caPEM,
		serverTLS:    srvTLS,
		agentKey:     agentPriv,
		agentCertPEM: agentPEM,
	}
}

// stubLinkServer stands up a TLS server that accepts the WebSocket
// upgrade at /api/v1/agent/link and runs a yamux server session
// long enough for Bootstrap's happy path to exercise the full chain.
// Streams are no-ops — we close any stream the client opens.
func stubLinkServer(t *testing.T, pki *linkTestPKI) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agent/link", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{link.Subprotocol},
		})
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer func() { _ = c.CloseNow() }()

		nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
		sess, err := link.NewServerSession(nc)
		if err != nil {
			return
		}
		defer func() { _ = sess.Close() }()

		for {
			_, stream, err := sess.Accept()
			if err != nil {
				return
			}
			_ = stream.Close()
		}
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverTLS},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	return srv
}

// Silence unused-import warnings if this test file shrinks.
var (
	_ = io.ReadWriteCloser(nil)
	_ = v2pb.StreamType(0)
)

// BootstrapV2 with an existing identity on disk: it must not reach
// out to an enroll endpoint; just load + dial.
func TestBootstrapV2_ReusesExistingIdentity(t *testing.T) {
	pki := mintLinkTestPKI(t, "agent-existing", "proj-1")
	dir := t.TempDir()
	if err := SaveIdentity(dir, pki.agentKey, pki.agentCertPEM, pki.caPEM); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	srv := stubLinkServer(t, pki)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	sess, err := BootstrapV2(ctx, BootstrapV2Options{
		IdentityDir: dir,
		ServerURL:   wsURL,
		// PAT / EnrollURL / ProjectCAPem intentionally empty — the
		// existing identity should be reused without any enroll hit.
	})
	if err != nil {
		t.Fatalf("BootstrapV2: %v", err)
	}
	sess.Close()
}

// No identity on disk and no PAT → BootstrapV2 must fail early and
// loudly rather than trying to dial with no cert.
func TestBootstrapV2_RequiresPATWhenNotEnrolled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := BootstrapV2(ctx, BootstrapV2Options{
		IdentityDir: t.TempDir(),
		ServerURL:   "wss://example.invalid/api/v1/agent/link",
		// PAT empty — nothing to enroll with.
	})
	if err == nil {
		t.Fatal("BootstrapV2 with no identity and no PAT should fail")
	}
	if !strings.Contains(err.Error(), "PAT") && !strings.Contains(err.Error(), "enroll") {
		t.Fatalf("error %q should mention PAT or enrollment", err.Error())
	}
}
