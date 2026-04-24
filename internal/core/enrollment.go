package core

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/storage"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// enrollSvc is the process-global handle to the enrollment service. It
// is set once at startup via SetEnrollment. The value is read without
// locking because the only writer is the startup path and the only
// readers are goroutines launched after startup.
var enrollSvc *enrollment.Service

// SetEnrollment registers the enrollment service used by the agent-
// facing TCP handshake. Called once from the server bootstrap.
func SetEnrollment(svc *enrollment.Service) {
	enrollSvc = svc
}

// enrollWaitTimeout is how long the server waits for the optional first
// AgentEnrollRequest before falling back to the legacy handshake. 2s is
// generous given the handshake runs immediately after TLS ClientHello.
const enrollWaitTimeout = 2 * time.Second

// EnrollmentOutcome summarises what happened during the optional
// enrollment dance. It's consumed by Handle to decide whether to accept
// the connection, and by the audit layer to record the event.
type EnrollmentOutcome struct {
	Attempted    bool
	Succeeded    bool
	AgentID      string
	SessionID    string
	ProjectID    string // resolved from the redeemed PAT / session — empty on failure
	Outcome      string // e.g. "success" / "invalid_secret" / "timeout"
	ErrorMessage string
}

// TryEnroll attempts the new PAT/session handshake on a freshly-
// accepted connection. If no enrollment frame arrives within
// enrollWaitTimeout, returns {Attempted: false} so the caller can
// proceed with the legacy flow. Errors only on I/O failures — rejected
// enrollments are reported via Outcome, not err.
func TryEnroll(client *AgentClient) (*EnrollmentOutcome, error) {
	if client == nil || client.conn == nil {
		return &EnrollmentOutcome{}, errors.New("core: nil client")
	}

	// Bound the wait. Clearing the deadline before we hand the connection
	// off to the rest of the pipeline is critical — a leftover deadline
	// would break the long-running shell/tunnel I/O.
	if err := client.conn.SetReadDeadline(time.Now().Add(enrollWaitTimeout)); err != nil {
		return &EnrollmentOutcome{}, err
	}
	env, err := client.codec.Recv()
	_ = client.conn.SetReadDeadline(time.Time{})

	if err != nil {
		if isTimeout(err) {
			return &EnrollmentOutcome{Attempted: false, Outcome: "legacy_no_enroll"}, nil
		}
		return &EnrollmentOutcome{}, err
	}

	req, ok := env.Payload.(*agentpb.Envelope_AgentEnrollRequest)
	if !ok {
		// The agent sent something else first — most likely an old build
		// that thinks the server will initiate. We can't easily put the
		// frame back; log and reject. When legacy agents are upgraded
		// this branch is unreachable.
		log.Warn("first frame from %s was %T, not AgentEnrollRequest — rejecting",
			client.conn.RemoteAddr(), env.Payload)
		return &EnrollmentOutcome{
			Attempted: true, Outcome: "legacy_wrong_first_frame",
			ErrorMessage: "first frame must be AgentEnrollRequest",
		}, nil
	}

	if enrollSvc == nil {
		// The server wasn't configured with enrollment but the agent
		// sent one anyway. Politely reject so the agent can fall back.
		_ = client.codec.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_AgentEnrollResponse{
				AgentEnrollResponse: &agentpb.AgentEnrollResponse{
					Error: "enrollment not configured on this server",
				},
			},
		})
		return &EnrollmentOutcome{Attempted: true, Outcome: "unconfigured"}, nil
	}

	rctx := enrollment.RedeemContext{
		ClientIP:    remoteIPOf(client.conn),
		MachineID:   req.AgentEnrollRequest.MachineId,
		Hostname:    req.AgentEnrollRequest.Hostname,
		AgentPubKey: req.AgentEnrollRequest.Pubkey,
	}

	result, redeemErr := redeemByPrefix(context.Background(), req.AgentEnrollRequest.Credential, rctx)

	// Build the response envelope. We always send exactly one envelope
	// back so the agent's own state machine can synchronise.
	resp := &agentpb.AgentEnrollResponse{}
	if result.Outcome == "success" {
		resp.AgentId = result.AgentID
		resp.SessionToken = result.SessionPlaintext
		resp.SessionExpiresAt = result.SessionExpiresAt.Unix()
		resp.RecommendedRenewAt = result.SessionExpiresAt.Add(-enrollment.RenewGrace).Unix()
		// Phase 4 PKI: optional leaf cert + CA chain. Empty when PKI
		// isn't configured; agents tolerate the empty-string form.
		resp.CertPem = result.CertPEM
		resp.CaPem = result.CAPem

		// Automatic Mesh Bootstrap: if the server has mesh enabled,
		// tell the agent how to join the overlay.
		if Ctx != nil && Ctx.Mesh != nil {
			if m, ok := Ctx.Mesh.(*mesh.Node); ok {
				resp.MeshPsk = m.PSK()
				resp.MeshPeers = m.AdvertisedAddrs()
				resp.MeshProjectId = m.ProjectID() // I need to expose ProjectID() in mesh.Node
			}
		}
	} else {
		resp.Error = result.Outcome
	}
	if err := client.codec.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_AgentEnrollResponse{AgentEnrollResponse: resp},
	}); err != nil {
		return &EnrollmentOutcome{}, err
	}

	recordAgentConnection(result, rctx.ClientIP, "tls_direct", "agent.enroll")

	outcome := &EnrollmentOutcome{
		Attempted: true,
		Succeeded: result.Outcome == "success",
		AgentID:   result.AgentID,
		SessionID: result.SessionID,
		Outcome:   result.Outcome,
	}
	// Surface the project id so callers (Handle) can stamp it on the
	// AgentClient before host/session persistence runs — the new
	// unified-ingress path has no TCPServer to fall back to. The
	// lookup is cheap (indexed on agent_id) and the DB is the source
	// of truth.
	if outcome.Succeeded && result.AgentID != "" && Ctx != nil && Ctx.Storage != nil {
		if sess, err := Ctx.Storage.AgentSessions().GetActive(context.Background(), result.AgentID); err == nil && sess != nil {
			outcome.ProjectID = sess.ProjectID
		}
	}
	if redeemErr != nil {
		outcome.ErrorMessage = redeemErr.Error()
	}
	return outcome, nil
}

// redeemByPrefix dispatches to RedeemPAT or RedeemSession based on the
// credential prefix. Hides the prefix-detection logic from the caller.
func redeemByPrefix(ctx context.Context, raw string, rctx enrollment.RedeemContext) (*enrollment.RedeemResult, error) {
	switch {
	case strings.HasPrefix(raw, enrollment.PATPrefix):
		return enrollSvc.RedeemPAT(ctx, raw, rctx)
	case strings.HasPrefix(raw, enrollment.SessionPrefix):
		return enrollSvc.RedeemSession(ctx, raw, rctx)
	default:
		return &enrollment.RedeemResult{Outcome: "malformed"}, enrollment.ErrMalformed
	}
}

// recordAgentConnection writes an agent lifecycle event (enroll /
// reconnect / disconnect) into the unified activities log. Resolves
// the project id via the enrolled agent session so cross-project
// filtering in the UI stays scoped correctly.
func recordAgentConnection(result *enrollment.RedeemResult, clientIP, transport, action string) {
	if Ctx == nil || Ctx.Storage == nil {
		return
	}
	outcome := storage.OutcomeSuccess
	if result.Outcome != "success" {
		outcome = storage.OutcomeDenied
		action += "_failed"
	}

	// Look up project id for successful events; on failure we keep it
	// global so the security timeline sees every attempt in one place.
	var projectID *string
	if result.Outcome == "success" && result.AgentID != "" {
		if sess, err := Ctx.Storage.AgentSessions().GetActive(context.Background(), result.AgentID); err == nil && sess != nil {
			pid := sess.ProjectID
			projectID = &pid
		}
	}

	meta := map[string]any{
		"transport": transport,
		"reason":    result.Outcome,
	}
	if result.SessionID != "" {
		meta["session_id"] = result.SessionID
	}

	activity.Record(activity.Input{
		ProjectID:   projectID,
		ActorType:   storage.ActorTypeAgent,
		ActorUser:   result.AgentID,
		ActorIP:     clientIP,
		Category:    storage.CategoryAgent,
		Action:      action,
		TargetType:  "agent",
		TargetID:    result.AgentID,
		TargetLabel: result.AgentID,
		Outcome:     outcome,
		SessionID:   result.SessionID,
		Meta:        meta,
	})
}

// isTimeout tells a generic TLS / TCP read timeout apart from a real
// read error. modernc.org/sqlite isn't involved here; this is the
// standard net.Error idiom.
func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// remoteIPOf extracts an IP-only string from a net.Conn's remote
// address. Falls back to the raw string if parsing fails.
func remoteIPOf(c net.Conn) string {
	if c == nil {
		return ""
	}
	addr := c.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// handleSessionRenew services an in-band SessionRenewRequest. We reuse
// enrollment.Service.RedeemSession because the semantics are identical:
// validate the echoed session_token, rotate to a new generation, return
// the plaintext to the agent. The difference vs reconnect is purely
// transport — the agent keeps the TLS connection open and its running
// PTYs / tunnels survive the rotation.
func handleSessionRenew(client *AgentClient, requestID string, req *agentpb.SessionRenewRequest) {
	resp := &agentpb.SessionRenewResponse{}
	defer func() {
		out := &agentpb.Envelope{
			Version:   1,
			Timestamp: time.Now().UnixNano(),
			RequestId: requestID,
			Payload:   &agentpb.Envelope_SessionRenewResponse{SessionRenewResponse: resp},
		}
		if err := client.codec.Send(out); err != nil {
			log.Warn("session renew response send: %v", err)
		}
	}()

	if enrollSvc == nil {
		resp.Error = "enrollment not configured"
		return
	}
	if req == nil || req.CurrentSessionToken == "" {
		resp.Error = "missing current session token"
		return
	}

	result, err := enrollSvc.RedeemSession(context.Background(), req.CurrentSessionToken,
		enrollment.RedeemContext{
			ClientIP: remoteIPOf(client.conn),
			// hostname / machine_id aren't needed for rotation — the
			// session row already has them from enrollment.
		})
	if err != nil {
		resp.Error = "renew failed"
		log.Warn("session renew error for %s: %v", client.OnelineDesc(), err)
		return
	}
	if result.Outcome != "success" {
		resp.Error = result.Outcome
		log.Info("session renew rejected for %s: %s", client.OnelineDesc(), result.Outcome)
		return
	}

	resp.SessionToken = result.SessionPlaintext
	resp.SessionExpiresAt = result.SessionExpiresAt.Unix()
	resp.RecommendedRenewAt = result.SessionExpiresAt.Add(-enrollment.RenewGrace).Unix()
	resp.CertPem = result.CertPEM
	resp.CaPem = result.CAPem

	// Record the rotation event so the audit trail reflects when a
	// long-running agent rotated its session without reconnecting.
	recordAgentConnection(result, remoteIPOf(client.conn), "tls_direct", "agent.reconnect")
}
