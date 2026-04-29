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
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/activity"
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
	linkStart := time.Now()
	if c.Request.TLS == nil || len(c.Request.TLS.PeerCertificates) == 0 {
		log.L.Warn("link.cert_missing",
			"remote_addr", c.Request.RemoteAddr,
			"client_ip", c.ClientIP(),
		)
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
		log.L.Warn("link.cert_verify_failed",
			"remote_addr", c.Request.RemoteAddr,
			"client_ip", c.ClientIP(),
			"error", err.Error(),
		)
		c.String(http.StatusUnauthorized, "agent link: client cert verification failed: %s", err)
		return
	}

	agentID, projectID, err := parseAgentSANs(leaf)
	if err != nil {
		log.L.Warn("link.san_parse_failed",
			"remote_addr", c.Request.RemoteAddr,
			"client_ip", c.ClientIP(),
			"error", err.Error(),
		)
		c.String(http.StatusBadRequest, "agent link: %s", err)
		return
	}

	// Approval gate. The cert chain proves "this agent_id was issued
	// by the project CA", but issuance happens on PAT redeem — a
	// leaked PAT can clear that bar. We require the operator to have
	// flipped the host to `approved` before letting the link open;
	// `pending` agents are turned away with a recognizable HTTP code
	// (425 Too Early) so the agent client can drive a friendly retry
	// loop. `rejected` agents get 403 — the cert is dead from the
	// server's perspective.
	if h.db != nil {
		host, lookupErr := h.db.Hosts().GetByAgentID(c.Request.Context(), agentID)
		switch {
		case lookupErr != nil && !errors.Is(lookupErr, storage.ErrNotFound):
			log.L.Warn("link.host_lookup_failed",
				"agent_id", agentID,
				"error", lookupErr.Error(),
			)
			c.String(http.StatusInternalServerError, "agent link: host lookup")
			return
		case lookupErr == nil:
			switch host.ApprovalStatus {
			case storage.HostApprovalRejected:
				log.L.Warn("link.rejected_by_admin",
					"agent_id", agentID,
					"project_id", projectID,
					"client_ip", c.ClientIP(),
					"decided_by", host.ApprovalDecidedBy,
				)
				c.Header("X-Platypus-Approval-Status", "rejected")
				c.String(http.StatusForbidden, "agent link: enrollment rejected by administrator")
				return
			case storage.HostApprovalPending:
				log.L.Info("link.pending_approval",
					"agent_id", agentID,
					"project_id", projectID,
					"client_ip", c.ClientIP(),
				)
				c.Header("X-Platypus-Approval-Status", "pending")
				c.String(http.StatusTooEarly, "agent link: awaiting admin approval; retry later")
				return
			}
		}
		// ErrNotFound on lookup means no hosts row was inserted at
		// enroll time (unlikely now that we always Upsert) — fall
		// through to accept the link rather than blocking; the
		// background SysInfo refresh will re-run Upsert on connect
		// and catch up the row.
	}

	wsConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		Subprotocols: []string{link.Subprotocol},
	})
	if err != nil {
		log.L.Warn("link.ws_upgrade_failed",
			"agent_id", agentID,
			"project_id", projectID,
			"client_ip", c.ClientIP(),
			"error", err.Error(),
		)
		return
	}
	// CloseNow on error paths; on normal path the yamux Close
	// cascades into a graceful WS close via NetConn.
	defer func() { _ = wsConn.CloseNow() }()

	nc := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
	sess, err := link.NewServerSession(nc)
	if err != nil {
		log.L.Warn("link.yamux_init_failed",
			"agent_id", agentID,
			"project_id", projectID,
			"client_ip", c.ClientIP(),
			"error", err.Error(),
		)
		return
	}
	defer func() { _ = sess.Close() }()

	// Register, displacing any stale prior session. Close the
	// displaced one so its accept loop unwinds.
	linkSessionID, prev := h.svc.Register(agentID, sess)
	sess.SetLinkSessionID(linkSessionID)
	if prev != nil {
		_ = prev.Close()
		log.L.Warn("link.displaced",
			"agent_id", agentID,
			"project_id", projectID,
			"link_session_id", linkSessionID,
		)
	}
	defer func() {
		h.svc.Unregister(agentID, sess)
		log.L.Info("link.unregistered",
			"agent_id", agentID,
			"project_id", projectID,
			"link_session_id", linkSessionID,
			"elapsed_ms", time.Since(linkStart).Milliseconds(),
		)
	}()

	log.L.Info("link.connected",
		"agent_id", agentID,
		"project_id", projectID,
		"link_session_id", linkSessionID,
		"remote_addr", c.Request.RemoteAddr,
		"client_ip", c.ClientIP(),
	)

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

		// Materialise a sessions row so the per-host UI mounts its
		// Terminal / Files tabs (TerminalTab gates on a live session
		// and renders "No live session" otherwise) and the project-
		// wide Sessions list shows the linked agent. Stamped
		// disconnected_at on the deferred path below so historical
		// queries reflect actual disconnect time, not "still live".
		if sessionID := h.startLiveSession(c, agentID, projectID); sessionID != "" {
			defer h.endLiveSession(sessionID, agentID, projectID)
		}
	}

	for {
		hdr, stream, err := sess.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.L.Info("link.disconnected",
					"agent_id", agentID,
					"project_id", projectID,
					"link_session_id", linkSessionID,
					"reason", "peer_eof",
					"elapsed_ms", time.Since(linkStart).Milliseconds(),
				)
				return
			}
			// Treat "shutdown via our own Close" as a clean exit.
			if c.Request.Context().Err() != nil {
				log.L.Info("link.disconnected",
					"agent_id", agentID,
					"project_id", projectID,
					"link_session_id", linkSessionID,
					"reason", "ctx_cancelled",
					"elapsed_ms", time.Since(linkStart).Milliseconds(),
				)
				return
			}
			log.L.Warn("link.accept_failed",
				"agent_id", agentID,
				"project_id", projectID,
				"link_session_id", linkSessionID,
				"error", err.Error(),
				"elapsed_ms", time.Since(linkStart).Milliseconds(),
			)
			return
		}
		log.L.Debug("link.stream_accept",
			"agent_id", agentID,
			"project_id", projectID,
			"link_session_id", linkSessionID,
			"stream_type", hdr.GetType().String(),
			"correlation_id", hdr.GetCorrelationId(),
		)
		go h.dispatchStream(agentID, hdr, stream)
	}
}

// startLiveSession resolves the host row backing this agent and
// inserts a sessions row with disconnected_at=NULL. Returns the new
// session_id (or "" on any failure — failures are logged and don't
// affect the link, since the live session is purely a UI affordance:
// the actual RPC plumbing keys off agent_id via AgentLinkService).
//
// Two failure paths to be aware of:
//   - host row missing: enroll-time Upsert may not have settled.
//     Skip the insert; the next reconnect picks it up.
//   - session insert fails: log and continue. The link still works,
//     just without UI surfacing.
func (h *AgentLinkHandler) startLiveSession(c *gin.Context, agentID, projectID string) string {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	host, err := h.db.Hosts().GetByAgentID(ctx, agentID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			log.Warn("agent link: lookup host for session insert (agent=%s): %v", agentID, err)
		}
		return ""
	}

	sess := &storage.Session{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		IngressAddr: PublicAddr,
		HostID:      host.ID,
		RemoteAddr:  c.Request.RemoteAddr,
		Version:     "v2",
		ConnectedAt: time.Now().UTC(),
	}
	if err := h.db.Sessions().Insert(ctx, sess); err != nil {
		log.Warn("agent link: insert session (agent=%s host=%s): %v", agentID, host.ID, err)
		return ""
	}
	// Audit row: the agent itself is the principal here (no human
	// user is on the request). Captures connect IP for forensic
	// review of "which IP did agent X connect from?".
	pid := projectID
	activity.Record(activity.Input{
		ProjectID:   &pid,
		ActorType:   storage.ActorTypeAgent,
		ActorUser:   agentID,
		ActorIP:     c.ClientIP(),
		ActorUA:     c.Request.UserAgent(),
		Category:    storage.CategorySession,
		Action:      "session.start",
		TargetType:  "agent",
		TargetID:    agentID,
		TargetLabel: host.ID,
		SessionID:   sess.ID,
		At:          sess.ConnectedAt,
	})
	return sess.ID
}

// endLiveSession stamps disconnected_at on the session row started by
// startLiveSession. Decoupled from the request context so a quick
// shutdown (server SIGTERM) still records the disconnect — uses a
// fresh background context with a short bound.
func (h *AgentLinkHandler) endLiveSession(sessionID, agentID, projectID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closedAt := time.Now().UTC()
	if err := h.db.Sessions().MarkDisconnected(ctx, sessionID); err != nil {
		log.L.Warn("link.session_record.close_failed",
			"session_record_id", sessionID,
			"error", err.Error(),
		)
		return
	}
	log.L.Info("link.session_record.closed",
		"session_record_id", sessionID,
	)
	// Audit row counterpart to session.start. We deliberately use
	// the package-level recorder (not RecordSystemActivity, which
	// needs a gin ctx) because we're past the request lifecycle.
	pid := projectID
	activity.RecordWithContext(ctx, activity.Input{
		ProjectID:  &pid,
		ActorType:  storage.ActorTypeAgent,
		ActorUser:  agentID,
		Category:   storage.CategorySession,
		Action:     "session.end",
		TargetType: "agent",
		TargetID:   agentID,
		SessionID:  sessionID,
		At:         closedAt,
	})
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
		BuildVersion:    s.BuildVersion,
		Commit:          s.Commit,
		BuildDate:       s.BuildDate,
		ProtocolVersion: s.ProtocolVersion,
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
