package api

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
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
	// db is optional; when set, the handler refreshes the matching
	// hosts row with the agent's latest SysInfo snapshot shortly
	// after each successful link connect. Kept optional so tests
	// that exercise the auth / ws plumbing don't need a DB.
	db *storage.DB
}

func NewAgentLinkHandler(svc *core.AgentLinkService, caPoolFn CertPoolFunc) *AgentLinkHandler {
	return &AgentLinkHandler{svc: svc, caPoolFn: caPoolFn}
}

// WithDB enables post-connect host info refresh. Call this during
// bootstrap when the DB is available; repeated calls replace the
// handle so wiring is idempotent.
func (h *AgentLinkHandler) WithDB(db *storage.DB) *AgentLinkHandler {
	h.db = db
	return h
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

	// Fire-and-forget: fetch a fresh SysInfo snapshot and upsert the
	// host row so the Web UI reflects this reconnect within a few
	// seconds. Errors are logged but don't affect the link — the
	// agent is already serving RPCs to end users.
	if h.db != nil {
		go h.refreshHostFromSysInfo(agentID, projectID)
		// Long-lived heartbeat: bump hosts.last_seen_at on a tick so
		// the Web UI's "online" presence dot (60s lookback against
		// last_seen_at; see desktop/.../lib/time.ts:isOnline) stays
		// green for the entire lifetime of a quiet link, not just
		// the first 60s after connect. Stops automatically when the
		// gin request context cancels (which happens on link drop).
		heartbeatCtx, stopHeartbeat := context.WithCancel(c.Request.Context())
		defer stopHeartbeat()
		go h.heartbeatHostLastSeen(heartbeatCtx, agentID)
	}

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

// heartbeatHostLastSeen ticks every hostHeartbeatInterval and bumps
// hosts.last_seen_at for the linked agent. Returns when ctx cancels
// (i.e. the link drops). Cheap one-row UPDATE per tick — keeping the
// interval comfortably under the frontend's 60s online window means
// the presence dot stays green even when no RPC traffic flows.
func (h *AgentLinkHandler) heartbeatHostLastSeen(ctx context.Context, agentID string) {
	t := time.NewTicker(hostHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := h.db.Hosts().TouchLastSeen(ctx, agentID, now); err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					// Enroll-time Upsert may still be in flight on
					// the very first tick after a fresh enrollment;
					// just try again next tick.
					continue
				}
				log.Debug("agent link: heartbeat last_seen for %s: %v", agentID, err)
			}
		}
	}
}

// hostHeartbeatInterval is comfortably below the Web UI's 60s
// online-presence window so a single missed tick (network blip, slow
// SQL write) doesn't flip the dot to grey.
const hostHeartbeatInterval = 20 * time.Second

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

// refreshHostFromSysInfo asks the freshly-connected agent for its
// SysInfo snapshot and merges the result into the hosts row. The
// call is bounded (5s) so a misbehaving agent can't leak a goroutine
// forever. If no host row exists yet for this agent (fresh
// enrollment on an older server, or the enroll-time upsert is still
// in flight) the Upsert will fall through to the insert path.
func (h *AgentLinkHandler) refreshHostFromSysInfo(agentID, projectID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := core.CallAgentRPC(ctx, h.svc, agentID, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_SysInfo{SysInfo: &v2pb.SysInfoRequest{}},
	})
	if err != nil {
		log.Debug("agent link: sys info refresh failed for %s: %v", agentID, err)
		return
	}
	s := resp.GetSysInfo()
	if s == nil {
		return
	}

	machineID := s.MachineId
	fingerprint := s.MachineId
	if fingerprint == "" {
		fingerprint = "fp-agent-" + agentID
		machineID = ""
	} else if strings.HasPrefix(fingerprint, "fp-") {
		machineID = ""
	}

	ident := &storage.HostIdentity{
		ProjectID:       projectID,
		MachineID:       machineID,
		Fingerprint:     fingerprint,
		Hostname:        s.Hostname,
		OS:              coalesce(s.Platform, s.Os),
		SeenAt:          time.Now().UTC(),
		AgentID:         agentID,
		Arch:            s.Arch,
		Platform:        s.Platform,
		PlatformFamily:  s.PlatformFamily,
		PlatformVersion: s.PlatformVersion,
		KernelVersion:   s.KernelVersion,
		CPUModel:        s.CpuModel,
		NumCPU:          int(s.NumCpu),
		MemTotalBytes:   int64(s.MemTotal),
		CurrentUser:     s.CurrentUser,
		Timezone:        s.Timezone,
		PrimaryIP:       s.PrimaryIp,
		PrimaryMAC:      s.PrimaryMac,
		BootTimeUnix:    int64(s.BootTimeUnix),
		AgentVersion:    s.AgentVersion,
		MachineType:     s.MachineType,
		ChassisType:     s.ChassisType,
		ProductVendor:   s.ProductVendor,
		ProductName:     s.ProductName,
		BIOSVendor:      s.BiosVendor,
		BIOSVersion:     s.BiosVersion,
		GPUSummary:      summarizeGPUs(s.Gpus),
	}
	if _, err := h.db.Hosts().Upsert(ctx, ident); err != nil {
		log.Warn("agent link: host upsert failed: agent=%s err=%v", agentID, err)
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// summarizeGPUs builds a short "vendor model; vendor model" blurb
// from the first few GPUInfo entries. Empty slices return "". The
// full live list stays in the SysInfo RPC — this denormalized string
// only exists so the hosts list can render a GPU column without
// issuing a per-row RPC.
func summarizeGPUs(gpus []*v2pb.GPUInfo) string {
	const maxEntries = 3
	parts := make([]string, 0, maxEntries)
	for _, g := range gpus {
		if g == nil {
			continue
		}
		label := strings.TrimSpace(strings.TrimSpace(g.Vendor) + " " + strings.TrimSpace(g.Model))
		if label == "" {
			continue
		}
		parts = append(parts, label)
		if len(parts) >= maxEntries {
			break
		}
	}
	return strings.Join(parts, "; ")
}
