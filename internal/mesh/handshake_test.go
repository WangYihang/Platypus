package mesh

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"sync"
	"testing"
	"time"

)

// pipeConn is a minimal in-memory duplex pipe. Each half is an io.Pipe
// so blocking semantics mirror a real socket well enough for handshake
// timing tests.
type pipeConn struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (p *pipeConn) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeConn) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeConn) Close() error                { p.r.Close(); p.w.Close(); return nil }

func newDuplex() (*pipeConn, *pipeConn) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	return &pipeConn{r: ar, w: aw}, &pipeConn{r: br, w: bw}
}

// mustIdentity mints a fresh cert-bound mesh Identity for a test.
// The leaf cert is signed by the package-shared test CA (see
// testhelpers_ca_test.go), so the full production cert-binding
// path (LoadIdentityFromCert + CertPEM + SAN-derived NodeID) is
// exercised by every mesh test that constructs an Identity.
func mustIdentity(t *testing.T) *Identity {
	t.Helper()
	certPEM, keyPEM := mintLeafFromTestCA(t, randAgentID(t))
	id, err := LoadIdentityFromCert(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert: %v", err)
	}
	return id
}

func randomPSK(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return b
}

// runHandshake spins up both sides concurrently and returns the results.
func runHandshake(t *testing.T, clientID, serverID *Identity, clientPSK, serverPSK []byte) (*HandshakeResult, *HandshakeResult, error, error) {
	t.Helper()
	clientConn, serverConn := newDuplex()
	defer clientConn.Close()
	defer serverConn.Close()

	clientCodec := newEnvCodec(clientConn)
	serverCodec := newEnvCodec(serverConn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var (
		wg         sync.WaitGroup
		cRes, sRes *HandshakeResult
		cErr, sErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		cRes, cErr = PerformClientHandshake(ctx, clientCodec, clientID, clientPSK, []string{"127.0.0.1:1"})
	}()
	go func() {
		defer wg.Done()
		sRes, sErr = PerformServerHandshake(ctx, serverCodec, serverID, serverPSK, []string{"127.0.0.1:2"})
	}()
	wg.Wait()
	return cRes, sRes, cErr, sErr
}

func TestHandshakeHappyPath(t *testing.T) {
	psk := randomPSK(t)
	cID := mustIdentity(t)
	sID := mustIdentity(t)

	cRes, sRes, cErr, sErr := runHandshake(t, cID, sID, psk, psk)
	if cErr != nil || sErr != nil {
		t.Fatalf("handshake errors: client=%v server=%v", cErr, sErr)
	}
	if cRes.PeerNodeID != sID.NodeID {
		t.Fatalf("client got wrong peer: %q want %q", cRes.PeerNodeID, sID.NodeID)
	}
	if sRes.PeerNodeID != cID.NodeID {
		t.Fatalf("server got wrong peer: %q want %q", sRes.PeerNodeID, cID.NodeID)
	}
	if !bytes.Equal(cRes.PeerPublicKey, sID.PublicKey) {
		t.Fatal("client got wrong peer pubkey")
	}
	if !bytes.Equal(sRes.PeerPublicKey, cID.PublicKey) {
		t.Fatal("server got wrong peer pubkey")
	}
}

func TestHandshakePSKMismatchRejected(t *testing.T) {
	cID := mustIdentity(t)
	sID := mustIdentity(t)
	// Server has a different PSK → server's MAC check on Hello should fail.
	_, _, cErr, sErr := runHandshake(t, cID, sID, randomPSK(t), randomPSK(t))
	if sErr == nil {
		t.Fatal("expected server-side PSK mismatch error")
	}
	// Client may hang on recv until server closes, so we don't strictly
	// require cErr != nil — but if it's set it should be a context /
	// recv error, not success.
	_ = cErr
}

func TestHandshakeForgedNodeIDRejected(t *testing.T) {
	// Build an Identity whose NodeID field lies but whose Ed25519 +
	// X25519 keys are otherwise well-formed — makes sure the server
	// rejects on the HandshakePayload node_id/pubkey mismatch check
	// rather than bailing at the Noise layer.
	base := mustIdentity(t)
	forged := &Identity{
		NodeID:        "this-is-not-the-real-node-id-xx",
		PublicKey:     base.PublicKey,
		PrivateKey:    base.PrivateKey,
		X25519Private: base.X25519Private,
		X25519Public:  base.X25519Public,
	}
	psk := randomPSK(t)
	_, _, _, sErr := runHandshake(t, forged, mustIdentity(t), psk, psk)
	if sErr == nil {
		t.Fatal("expected server to reject forged NodeID")
	}
}

// TestHandshakeSelfIDRejected: an initiator claiming the same NodeID
// as the responder must be rejected — otherwise a compromised node
// could impersonate its peer on a loopback attack.
func TestHandshakeSelfIDRejected(t *testing.T) {
	psk := randomPSK(t)
	shared := mustIdentity(t)
	// Both sides have the SAME Identity. Server must notice the
	// peer claims its own NodeID in the payload and bail.
	_, _, _, sErr := runHandshake(t, shared, shared, psk, psk)
	if sErr == nil {
		t.Fatal("expected server to reject peer claiming its own NodeID")
	}
}
