package api

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// CertPoolFunc returns the live project-CA pool the handler should
// verify client certs against. Passing a function rather than a
// *x509.CertPool lets the server swap pools at runtime when
// operators rotate a project CA without restarting the process.
type CertPoolFunc func() *x509.CertPool

// AgentLinkHandler serves the v2 agent link endpoint. Agents dial
// here with an mTLS client certificate (issued by the project CA
// via POST /api/v1/agents/enroll); the handler verifies the chain,
// extracts agent_id from the URI SAN, upgrades to WebSocket, wraps
// the connection in a yamux server Session, and registers that
// Session in the process-wide AgentLinkService so other API
// handlers can open streams against the agent.
type AgentLinkHandler struct {
	svc      *core.AgentLinkService
	caPoolFn CertPoolFunc
}

func NewAgentLinkHandler(svc *core.AgentLinkService, caPoolFn CertPoolFunc) *AgentLinkHandler {
	return &AgentLinkHandler{svc: svc, caPoolFn: caPoolFn}
}

// Handle is the Gin handler.
//
//   - 401 when no client cert is presented
//   - 401 when the presented cert doesn't chain to the project CA
//   - 400 when the cert lacks a platypus://agent/<id> URI SAN
//   - on success: WS Upgrade, register Session, run accept loop
//
// The accept loop is a stub for now: every stream is closed
// immediately with a StreamReject referencing "not-yet-implemented".
// Subsequent commits wire up per-stream-type dispatch.
func (h *AgentLinkHandler) Handle(c *gin.Context) {
	if c.Request.TLS == nil || len(c.Request.TLS.PeerCertificates) == 0 {
		c.String(http.StatusUnauthorized, "agent link: client certificate required")
		return
	}
	leaf := c.Request.TLS.PeerCertificates[0]

	// Chain verification — TLS handshake may have accepted the cert
	// under ClientAuth: RequestClientCert (which does NOT verify), so
	// we must validate against the project CA pool ourselves.
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     h.caPoolFn(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		c.String(http.StatusUnauthorized, "agent link: client cert verification failed: %s", err)
		return
	}

	agentID, projectID, err := parseAgentSANs(leaf)
	if err != nil {
		c.String(http.StatusBadRequest, "agent link: %s", err)
		return
	}

	wsConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		Subprotocols: []string{link.Subprotocol},
	})
	if err != nil {
		log.Error("agent link: ws upgrade for %s: %v", agentID, err)
		return
	}
	// CloseNow on error paths; on normal path the yamux Close
	// cascades into a graceful WS close via NetConn.
	defer func() { _ = wsConn.CloseNow() }()

	nc := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
	sess, err := link.NewServerSession(nc)
	if err != nil {
		log.Error("agent link: yamux server for %s: %v", agentID, err)
		return
	}
	defer func() { _ = sess.Close() }()

	// Register, displacing any stale prior session. Close the
	// displaced one so its accept loop unwinds.
	if prev := h.svc.Register(agentID, sess); prev != nil {
		_ = prev.Close()
		log.Info("agent link: displaced stale session for %s", agentID)
	}
	defer h.svc.Unregister(agentID)

	log.Success("agent link: %s (project=%s) connected", agentID, projectID)

	for {
		hdr, stream, err := sess.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Info("agent link: %s disconnected", agentID)
				return
			}
			// Treat "shutdown via our own Close" as a clean exit.
			if c.Request.Context().Err() != nil {
				return
			}
			log.Warn("agent link: %s accept: %v", agentID, err)
			return
		}
		go h.dispatchStream(agentID, hdr, stream)
	}
}

// dispatchStream is the stub dispatcher until Phase II.C wires up
// per-stream-type handlers. We write a StreamReject back to the
// peer so the agent side knows this build doesn't understand the
// stream yet, then close.
func (h *AgentLinkHandler) dispatchStream(agentID string, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser) {
	defer func() { _ = stream.Close() }()

	rej := &v2pb.StreamReject{
		Code:    "unsupported_type",
		Message: fmt.Sprintf("agent link: no handler for %s yet", hdr.Type),
	}
	if err := link.WriteFrame(stream, rej); err != nil {
		log.Warn("agent link: %s reject write for %s: %v", agentID, hdr.Type, err)
	}
}

// parseAgentSANs walks the cert's URIs and returns (agent_id, project_id).
// The scheme + path format matches what pki.IssueAgentLeafFromCSR
// writes: platypus://agent/<id> and platypus://project/<id>.
func parseAgentSANs(leaf *x509.Certificate) (string, string, error) {
	var agentID, projectID string
	for _, u := range leaf.URIs {
		if u.Scheme != "platypus" {
			continue
		}
		switch u.Host {
		case "agent":
			agentID = strings.TrimPrefix(u.Path, "/")
		case "project":
			projectID = strings.TrimPrefix(u.Path, "/")
		}
	}
	if agentID == "" {
		return "", "", errors.New("client cert missing platypus://agent/<id> URI SAN")
	}
	if projectID == "" {
		return "", "", errors.New("client cert missing platypus://project/<id> URI SAN")
	}
	return agentID, projectID, nil
}

// RegisterV2AgentLinkRoute mounts the endpoint. No RBAC middleware —
// the mTLS chain IS the credential, and it's verified in-handler.
func RegisterV2AgentLinkRoute(engine *gin.Engine, h *AgentLinkHandler) {
	engine.GET("/api/v1/agent/link", h.Handle)
}
