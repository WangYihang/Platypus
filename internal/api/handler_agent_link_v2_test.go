package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
)

// agentLinkTestPKI mints a throwaway CA plus a server cert + client
// cert signed by it. Returns everything the test needs to stand up
// a TLS httptest.Server requiring mTLS.
type agentLinkTestPKI struct {
	caCert    *x509.Certificate
	caPEM     []byte
	caPool    *x509.CertPool
	serverTLS tls.Certificate
	clientTLS tls.Certificate
}

func mintAgentLinkPKI(t *testing.T, agentID, projectID string) *agentLinkTestPKI {
	t.Helper()

	caPub, caPriv, _ := ed25519.GenerateKey(rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "link-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server cert: SAN contains 127.0.0.1 so httptest's Listener passes hostname verification.
	srvPub, srvPriv, _ := ed25519.GenerateKey(rand.Reader)
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "link-test-server"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, srvPub, caPriv)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	srvTLS := tls.Certificate{
		Certificate: [][]byte{srvDER},
		PrivateKey:  srvPriv,
	}

	// Client cert: URI SAN identifies the agent + project, mimicking what
	// pki.IssueAgentLeafFromCSR writes in production.
	cliPub, cliPriv, _ := ed25519.GenerateKey(rand.Reader)
	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/" + projectID)
	cliTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{agentURI, projectURI},
	}
	cliDER, err := x509.CreateCertificate(rand.Reader, cliTmpl, caCert, cliPub, caPriv)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	cliTLS := tls.Certificate{
		Certificate: [][]byte{cliDER},
		PrivateKey:  cliPriv,
	}
	return &agentLinkTestPKI{
		caCert:    caCert,
		caPEM:     caPEM,
		caPool:    caPool,
		serverTLS: srvTLS,
		clientTLS: cliTLS,
	}
}

// Happy path: client dials with a valid cert, the handler extracts
// the agent_id from the URI SAN and registers the Session; a
// subsequent lookup through the service returns it.
func TestAgentLinkHandler_RegistersSessionOnConnect(t *testing.T) {
	pki := mintAgentLinkPKI(t, "agent-happy", "proj-1")
	svc := core.NewAgentLinkService()
	h := NewAgentLinkHandler(svc, func() *x509.CertPool { return pki.caPool })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentLinkRoute(r, h)

	srv := httptest.NewUnstartedServer(r)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverTLS},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{pki.clientTLS},
		RootCAs:      pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	sess, err := link.Dial(ctx, link.DialOptions{URL: wsURL, TLSConfig: clientTLS})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// The handler registers synchronously before entering the accept
	// loop, so the session should be visible immediately. Poll with a
	// short budget to absorb goroutine scheduling jitter.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := svc.Get("agent-happy"); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent-happy not registered in AgentLinkService after Dial")
}

// A client with no cert is rejected with 401 at the handler level.
func TestAgentLinkHandler_RejectsMissingClientCert(t *testing.T) {
	pki := mintAgentLinkPKI(t, "agent-1", "proj-1")
	svc := core.NewAgentLinkService()
	h := NewAgentLinkHandler(svc, func() *x509.CertPool { return pki.caPool })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentLinkRoute(r, h)

	srv := httptest.NewUnstartedServer(r)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverTLS},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	defer srv.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pki.caPool, MinVersion: tls.VersionTLS12},
		},
	}
	resp, err := client.Get(srv.URL + "/api/v1/agent/link")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", resp.StatusCode)
	}
}

// A client cert not signed by the trusted CA is rejected at 401.
func TestAgentLinkHandler_RejectsWrongCA(t *testing.T) {
	realPKI := mintAgentLinkPKI(t, "agent-1", "proj-1")
	rogue := mintAgentLinkPKI(t, "agent-rogue", "proj-evil")

	svc := core.NewAgentLinkService()
	h := NewAgentLinkHandler(svc, func() *x509.CertPool { return realPKI.caPool })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentLinkRoute(r, h)

	srv := httptest.NewUnstartedServer(r)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{realPKI.serverTLS},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Client presents the rogue cert; server trusts only realPKI.
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{rogue.clientTLS},
		RootCAs:      realPKI.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	_, err := link.Dial(ctx, link.DialOptions{URL: wsURL, TLSConfig: clientTLS})
	if err == nil {
		t.Fatal("Dial should have failed with untrusted client cert")
	}
}
