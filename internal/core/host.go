package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
)

// ComputeFingerprint produces a deterministic weak host fingerprint from
// hostname + sorted MAC addresses. Matches the algorithm used by the
// agent's internal/agent.fallbackFingerprint so a session that initially
// reports machine_id="fp-XYZ" (agent fallback) and later reports a real
// platform id can still be merged onto the same host row by fingerprint.
func ComputeFingerprint(hostname string, interfaces map[string]string) string {
	macs := make([]string, 0, len(interfaces))
	for _, mac := range interfaces {
		if mac != "" {
			macs = append(macs, mac)
		}
	}
	sort.Strings(macs)

	h := sha256.New()
	h.Write([]byte(hostname))
	h.Write([]byte{0})
	for _, m := range macs {
		h.Write([]byte(m))
		h.Write([]byte{0})
	}
	return "fp-" + hex.EncodeToString(h.Sum(nil))
}

// SplitAgentMachineID unwraps the single machine_id field the agent sends:
// an "fp-"-prefixed value is the agent's own fingerprint fallback, which
// the server demotes to MachineID="" so host-aggregation uses the
// fingerprint path. Anything else is treated as a real platform id.
func SplitAgentMachineID(reported string) (machineID string, isFallback bool) {
	if strings.HasPrefix(reported, "fp-") {
		return "", true
	}
	return reported, reported == ""
}

// UpsertHostForAgent writes the host identity the agent reported into
// storage under the right project, and stashes the resulting Host ID on
// the AgentClient so downstream code (session persistence, dispatch,
// etc.) can look it up without another round trip.
//
// When the TCPServer has no ProjectID set (today's legacy case — nothing
// writes the field yet), the "default" project is looked up by slug. If
// even that is missing (e.g. no bootstrap has happened), we log and skip
// — the session still works, just without a persisted Host row.
func UpsertHostForAgent(ctx context.Context, c *AgentClient) {
	if Ctx == nil || Ctx.Storage == nil {
		return
	}
	projectID, err := resolveProjectID(ctx, c.server)
	if err != nil {
		log.Warn("Host upsert skipped (no project): %s", err)
		return
	}
	machineID, _ := SplitAgentMachineID(c.MachineID)
	fingerprint := ComputeFingerprint(c.Hostname, c.NetworkInterfaces)

	host, err := Ctx.Storage.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   projectID,
		MachineID:   machineID,
		Fingerprint: fingerprint,
		Hostname:    c.Hostname,
		OS:          c.OS.String(),
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		log.Warn("Host upsert failed: %s", err)
		return
	}
	c.HostID = host.ID
	c.ProjectID = host.ProjectID

	// Fan out to /notify subscribers so the UI doesn't have to poll.
	BroadcastNotify(EventHostSeen, map[string]any{
		"project_id":           host.ProjectID,
		"host_id":              host.ID,
		"hostname":             host.Hostname,
		"fingerprint_fallback": host.FingerprintFallback,
	})
}

func resolveProjectID(ctx context.Context, s *TCPServer) (string, error) {
	if s != nil && s.ProjectID != "" {
		return s.ProjectID, nil
	}
	p, err := Ctx.Storage.Projects().GetBySlug(ctx, "default")
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", errors.New("no default project — bootstrap first")
		}
		return "", err
	}
	return p.ID, nil
}

// ingressAddrFor computes the host:port audit string persisted on the
// session row. During the unified-ingress transition the TCPServer still
// owns the port; once PR-E lands this falls back to the dispatcher's
// PublicAddr exposed via core.AgentService.
func ingressAddrFor(c *AgentClient) string {
	if c == nil || c.server == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.server.Host, c.server.Port)
}

// PersistSessionForAgent writes the session row. Requires HostID +
// ProjectID to already be populated (UpsertHostForAgent does this).
// Skips silently when storage is absent.
func PersistSessionForAgent(ctx context.Context, c *AgentClient) {
	if Ctx == nil || Ctx.Storage == nil {
		return
	}
	if c.HostID == "" || c.ProjectID == "" {
		// Host upsert was skipped (no default project etc.) — no safe
		// point to persist the session.
		return
	}
	ifacesJSON, _ := json.Marshal(c.NetworkInterfaces)
	err := Ctx.Storage.Sessions().Insert(ctx, &storage.Session{
		ID:             c.Hash,
		ProjectID:      c.ProjectID,
		IngressAddr:    ingressAddrFor(c),
		HostID:         c.HostID,
		Alias:          c.Alias,
		User:           c.User,
		RemoteAddr:     c.conn.RemoteAddr().String(),
		Version:        c.Version,
		Python2:        c.Python2,
		Python3:        c.Python3,
		InterfacesJSON: string(ifacesJSON),
		GroupDispatch:  c.GroupDispatch,
		ConnectedAt:    c.TimeStamp.UTC(),
	})
	if err != nil {
		log.Warn("Session persistence failed: %s", err)
		return
	}
	BroadcastNotify(EventSessionOpened, map[string]any{
		"project_id": c.ProjectID,
		"host_id":    c.HostID,
		"session_id": c.Hash,
	})
}

// MarkSessionDisconnected stamps disconnected_at on the session row.
// Called from DeleteAgentClient's disconnect path; idempotent so the
// duplicate-client rejection path can also call it safely.
func MarkSessionDisconnected(ctx context.Context, c *AgentClient) {
	if Ctx == nil || Ctx.Storage == nil || c == nil || c.Hash == "" {
		return
	}
	if err := Ctx.Storage.Sessions().MarkDisconnected(ctx, c.Hash); err != nil {
		log.Warn("MarkDisconnected(%s) failed: %s", c.Hash, err)
	}
	// Emit even if the repo call failed — the session is still gone
	// from runtime, and the UI will refetch on receiving the event.
	if c.ProjectID != "" && c.HostID != "" {
		BroadcastNotify(EventSessionClosed, map[string]any{
			"project_id": c.ProjectID,
			"host_id":    c.HostID,
			"session_id": c.Hash,
		})
	}
}
