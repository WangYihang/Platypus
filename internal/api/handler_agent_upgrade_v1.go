package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// AgentUpgradeHandler handles admin-triggered self-upgrade of a
// single agent. The flow is server-initiated: the admin POSTs a
// target version + channel, the server opens a
// STREAM_TYPE_AGENT_UPGRADE stream against the live link, drains
// progress frames, and waits for the agent to reach a terminal phase.
//
// We deliberately wait for completion (or first hard error) rather
// than fire-and-forget. Operators clicking the button want to know
// whether their fleet host actually picked up the new build before
// they move on. The HTTP request stays open for at most
// upgradeStreamTimeout; longer-running installs (very slow links)
// will still complete on the agent side, but the API response
// returns "in progress" and the audit trail is the source of truth.
type AgentUpgradeHandler struct {
	svc *core.AgentLinkService
}

// NewAgentUpgradeHandler binds the handler to the live link
// registry. Constructor lives next to its consumer in main.go so
// the wiring stays grep-able.
func NewAgentUpgradeHandler(svc *core.AgentLinkService) *AgentUpgradeHandler {
	return &AgentUpgradeHandler{svc: svc}
}

// upgradeStreamTimeout caps how long the HTTP request blocks waiting
// for the agent to reach a terminal phase. Generous enough that even
// a 100 MB binary on a 10 Mbit link finishes inside the window
// (~80s), with margin for slow disks and the post-rename flush.
// Beyond this, the agent's flow keeps running but the operator gets
// "in progress" and can poll the activity log.
const upgradeStreamTimeout = 5 * time.Minute

// upgradeRequest is the POST body. Both fields are optional:
//   - target_version="" means "current head of channel"
//   - channel=""        means "stable" (server default)
type upgradeRequest struct {
	TargetVersion string `json:"target_version"`
	Channel       string `json:"channel"`
}

// upgradeResponse summarises the outcome the operator sees in the
// browser. Fields are optional to keep the schema stable as new
// terminal phases get added.
type upgradeResponse struct {
	Status       string `json:"status"` // "exited" | "failed" | "in_progress"
	Phase        string `json:"phase"`  // last phase observed
	ResolvedVer  string `json:"resolved_version,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`    // only on failed
	ErrorMessage string `json:"error_message,omitempty"` // only on failed
	BytesDone    uint64 `json:"bytes_done,omitempty"`
	BytesTotal   uint64 `json:"bytes_total,omitempty"`
}

// Trigger handles POST /api/v1/projects/:pid/agents/:agent_id/upgrade.
// Gated by RequireAuth + RequireProjectRole(admin) at registration.
func (h *AgentUpgradeHandler) Trigger(c *gin.Context) {
	projectID := c.Param("pid")
	agentID := c.Param("agent_id")
	claims, _ := ClaimsFromContext(c)

	var body upgradeRequest
	// Body is optional: an empty POST means "channel=stable, latest".
	_ = c.ShouldBindJSON(&body)

	sess, sessionID, ok := h.svc.GetWithSessionID(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not connected"})
		return
	}

	channel := body.Channel
	if channel == "" {
		channel = "stable"
	}
	actor := "user:" + claims.UserID
	req := &v2pb.AgentUpgradeRequest{
		TargetVersion: body.TargetVersion,
		Channel:       channel,
		Actor:         actor,
	}
	metaBytes, err := proto.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal upgrade request"})
		return
	}

	// Audit "upgrade.start" up-front so an admin who closes the
	// browser tab still leaves a forensic trail.
	pid := projectID
	startedAt := time.Now().UTC()
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      "agent.upgrade.start",
		TargetType:  "agent",
		TargetID:    agentID,
		TargetLabel: agentID,
		Outcome:     storage.OutcomeSuccess,
		Meta: map[string]string{
			"target_version":  body.TargetVersion,
			"channel":         channel,
			"link_session_id": sessionID,
		},
		At: startedAt,
	})

	// Open the upgrade stream. This runs even when the request ctx
	// cancels (admin closed the tab) — the upgrade must not abort
	// mid-flight just because the operator looked away. Use the
	// background-derived timeout instead.
	ctx, cancel := withRequestOrTimeout(c.Request.Context(), upgradeStreamTimeout)
	defer cancel()
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_AGENT_UPGRADE, metaBytes,
		"upgrade-"+agentID+"-"+sessionID)
	if err != nil {
		log.Warn("agent.upgrade: open stream agent=%s err=%v", agentID, err)
		recordUpgradeOutcome(c, &pid, agentID, sessionID, "failed", "stream_open_failed", err.Error(), startedAt)
		c.JSON(http.StatusBadGateway, gin.H{"error": "open upgrade stream: " + err.Error()})
		return
	}
	defer func() { _ = stream.Close() }()

	final, drainErr := drainUpgradeProgress(ctx, stream)
	resp := upgradeResponse{
		Phase:        final.GetPhase().String(),
		ResolvedVer:  final.GetResolvedVersion(),
		BytesDone:    final.GetBytesDone(),
		BytesTotal:   final.GetBytesTotal(),
		ErrorCode:    final.GetErrorCode(),
		ErrorMessage: final.GetErrorMessage(),
	}
	// Status-code policy: 200 for every outcome where the REST layer
	// itself succeeded (the upgrade flow ran end-to-end and we have a
	// terminal phase, even if that phase is FAILED). The body's
	// `status` field carries the operational outcome; clients branch
	// on it. 5xx is reserved for "couldn't run the flow at all" —
	// agent offline, timeout, unknown phase. Keeps the JSON shape
	// uniform across success / failure paths and avoids forcing
	// frontends to special-case error-body parsing.
	switch {
	case drainErr != nil:
		resp.Status = "in_progress"
		// Timeout / link drop while still waiting. Most likely the
		// agent is mid-download on a slow link; the upgrade may yet
		// complete. Recorded as a separate "timeout" outcome so the
		// audit log can distinguish it from a hard fail.
		log.Info("agent.upgrade: drain timeout agent=%s err=%v", agentID, drainErr)
		recordUpgradeOutcome(c, &pid, agentID, sessionID, "in_progress",
			"drain_timeout", drainErr.Error(), startedAt)
		c.JSON(http.StatusAccepted, resp)
	case final.GetPhase() == v2pb.UpgradeProgress_PHASE_EXITING:
		resp.Status = "exited"
		recordUpgradeOutcome(c, &pid, agentID, sessionID, "exited", "", "", startedAt)
		c.JSON(http.StatusOK, resp)
	case final.GetPhase() == v2pb.UpgradeProgress_PHASE_FAILED:
		resp.Status = "failed"
		recordUpgradeOutcome(c, &pid, agentID, sessionID, "failed",
			final.GetErrorCode(), final.GetErrorMessage(), startedAt)
		c.JSON(http.StatusOK, resp)
	default:
		// Shouldn't happen — drainUpgradeProgress only returns nil
		// drainErr on a terminal phase. Defend against future
		// proto changes by surfacing the unknown phase instead of
		// silently 200ing.
		resp.Status = "unknown"
		log.Warn("agent.upgrade: unexpected non-terminal phase agent=%s phase=%v",
			agentID, final.GetPhase())
		c.JSON(http.StatusInternalServerError, resp)
	}
}

// drainUpgradeProgress reads UpgradeProgress frames until the agent
// reports a terminal phase or the context expires. Returns the most
// recent frame seen plus any drain error (typically a deadline).
//
// We read every frame so the server-side log carries the full
// progression — this is what the future SSE / WS streaming variant
// will plug into. For the synchronous REST endpoint only the
// terminal frame survives in the JSON response.
func drainUpgradeProgress(ctx context.Context, stream io.ReadWriteCloser) (*v2pb.UpgradeProgress, error) {
	var last v2pb.UpgradeProgress
	for {
		select {
		case <-ctx.Done():
			return &last, ctx.Err()
		default:
		}
		var p v2pb.UpgradeProgress
		if err := link.ReadFrame(stream, &p); err != nil {
			// Genuine read errors (link drop, EOF before terminal)
			// surface as drain errors so the caller records them
			// as in_progress / drain failure.
			if last.GetPhase() == v2pb.UpgradeProgress_PHASE_EXITING ||
				last.GetPhase() == v2pb.UpgradeProgress_PHASE_FAILED {
				// Already-terminal frames are a clean close.
				return &last, nil
			}
			return &last, err
		}
		last = p
		log.L.Debug("agent.upgrade.progress",
			"phase", p.GetPhase().String(),
			"bytes_done", p.GetBytesDone(),
			"bytes_total", p.GetBytesTotal(),
			"resolved_version", p.GetResolvedVersion(),
		)
		switch p.Phase {
		case v2pb.UpgradeProgress_PHASE_EXITING,
			v2pb.UpgradeProgress_PHASE_FAILED:
			return &last, nil
		}
	}
}

// recordUpgradeOutcome writes the matching "agent.upgrade.end" row
// to the activity log. Centralised so all four exit paths
// (exited / failed / drain_timeout / stream_open_failed) emit the
// same shape, which the UI's history tab relies on for grouping.
func recordUpgradeOutcome(
	c *gin.Context, projectID *string, agentID, sessionID string,
	status, errCode, errMsg string, startedAt time.Time,
) {
	now := time.Now().UTC()
	outcome := storage.OutcomeSuccess
	if status != "exited" {
		outcome = storage.OutcomeError
	}
	meta := map[string]string{
		"status":          status,
		"link_session_id": sessionID,
		"duration_ms":     fmt.Sprintf("%d", now.Sub(startedAt).Milliseconds()),
	}
	if errCode != "" {
		meta["error_code"] = errCode
	}
	if errMsg != "" {
		meta["error_message"] = errMsg
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   projectID,
		Category:    storage.CategoryAdmin,
		Action:      "agent.upgrade.end",
		TargetType:  "agent",
		TargetID:    agentID,
		TargetLabel: agentID,
		Outcome:     outcome,
		Meta:        meta,
		At:          now,
	})
}

// withRequestOrTimeout returns a derived context that cancels when
// either the parent (typically the request ctx) cancels or
// the timeout elapses, whichever first. Replaces the bare
// context.WithTimeout because the request context cancels the
// instant the browser tab closes — and we want the upgrade flow to
// continue beyond that, capped only by the absolute timeout.
func withRequestOrTimeout(_ context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	// Detach: derive from Background so a closed browser tab
	// doesn't yank the rug out from under a mid-install agent.
	return context.WithTimeout(context.Background(), d)
}

// errAgentNotConnected lets a future "upgrade many" caller distinguish
// the "no live link" case from generic Open() failures without
// string-matching the error message.
var errAgentNotConnected = errors.New("agent not connected")

// _ = errAgentNotConnected to keep govet happy until the batch caller
// lands; remove this when consumed.
var _ = errAgentNotConnected

// RegisterV1AgentUpgradeRoutes mounts POST
// /api/v1/projects/:pid/agents/:agent_id/upgrade. Gated by
// RequireAuth + RequireProjectRole(admin) — only an admin in the
// project can trigger a self-upgrade on one of its agents.
//
// The agent path param has to be `:agent_id` to share the
// `/api/v1/projects/:pid/agents/:agent_id/...` prefix with the fs/
// terminal/exec handlers — Gin's trie refuses to register two
// distinct wildcard names under the same prefix and panics on
// startup if it sees `:aid` here alongside `:agent_id` elsewhere.
func RegisterV1AgentUpgradeRoutes(engine *gin.Engine, h *AgentUpgradeHandler, rbac *RBAC) {
	admin := engine.Group("/api/v1/projects/:pid/agents")
	admin.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		admin.POST("/:agent_id/upgrade", h.Trigger)
	}
}
