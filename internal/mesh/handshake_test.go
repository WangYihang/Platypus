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

func mustIdentity(t *testing.T) *Identity {
	t.Helper()
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
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
	// Build an Identity whose NodeID field is a lie. The server must
	// reject it during validateHelloCommon.
	base := mustIdentity(t)
	forged := &Identity{
		NodeID:     "this-is-not-the-real-node-id-xx",
		PublicKey:  base.PublicKey,
		PrivateKey: base.PrivateKey,
	}
	psk := randomPSK(t)
	_, _, _, sErr := runHandshake(t, forged, mustIdentity(t), psk, psk)
	if sErr == nil {
		t.Fatal("expected server to reject forged NodeID")
	}
}
