package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"

	"github.com/coder/websocket"
)

// LinkPath is the HTTP path the mesh upgrade endpoint serves under.
// Both the server's REST router and the agent's peer listener mount
// the same handler at this path.
const LinkPath = "/api/v1/mesh/link"

// CertPoolFn yields the project-CA pool against which inbound mesh
// peer client certs are verified. A function (not a static pool) so
// callers can swap pools at runtime when a CA rotates without a
// process restart.
type CertPoolFn func() *x509.CertPool

// LinkHandler upgrades inbound mesh peer connections. Plain
// http.Handler (no gin) so the same instance plugs into the server's
// REST router and the agent's standalone peer listener.
type LinkHandler struct {
	node     *Node
	caPoolFn CertPoolFn
}

func NewLinkHandler(node *Node, caPoolFn CertPoolFn) *LinkHandler {
	return &LinkHandler{node: node, caPoolFn: caPoolFn}
}

func (h *LinkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "mesh link: client certificate required", http.StatusUnauthorized)
		return
	}
	leaf := r.TLS.PeerCertificates[0]
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     h.caPoolFn(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		http.Error(w, "mesh link: client cert verification failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	peerNodeID, err := MeshNodeIDFromCert(leaf)
	if err != nil {
		http.Error(w, "mesh link: "+err.Error(), http.StatusBadRequest)
		return
	}
	if peerNodeID == h.node.LocalNodeID() {
		http.Error(w, "mesh link: peer is self", http.StatusBadRequest)
		return
	}
	peerPubkey, ok := leaf.PublicKey.(ed25519.PublicKey)
	if !ok {
		http.Error(w, "mesh link: peer cert must use Ed25519 key", http.StatusBadRequest)
		return
	}

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{LinkSubprotocol},
	})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.CloseNow() }()

	ctx := context.Background()
	nc := NetConnFromWebSocket(ctx, wsConn)
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	h.node.AdoptStream(ctx, nc, peerNodeID, peerPubkey, leafPEM)
}

// MeshNodeIDFromCert returns "<id>" for platypus://agent/<id> URI SANs
// and "server-<id>" for platypus://server/<id>. Used by the inbound
// handler and the outbound dialer's verify hook.
func MeshNodeIDFromCert(leaf *x509.Certificate) (string, error) {
	for _, u := range leaf.URIs {
		if u.Scheme != "platypus" {
			continue
		}
		id := strings.TrimPrefix(u.Path, "/")
		switch u.Host {
		case "agent":
			if id == "" {
				return "", errors.New("platypus://agent/ SAN has empty id")
			}
			return id, nil
		case "server":
			if id == "" {
				return "", errors.New("platypus://server/ SAN has empty id")
			}
			return "server-" + id, nil
		}
	}
	return "", errors.New("peer cert missing platypus://agent/<id> or platypus://server/<id> URI SAN")
}
