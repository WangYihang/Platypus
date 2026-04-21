package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// EnsureListenerRow makes sure the persistent listeners row exists for s
// before any session persistence attempts to reference it. TCPServers
// created through the legacy config path don't get a row at startup; we
// upsert-on-first-use here so session inserts don't FK-fail.
func EnsureListenerRow(ctx context.Context, s *TCPServer) error {
	if Ctx == nil || Ctx.Storage == nil || s == nil {
		return nil
	}
	if _, err := Ctx.Storage.Listeners().GetByID(ctx, s.Hash); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	projectID, err := resolveProjectID(ctx, s)
	if err != nil {
		return err
	}
	return Ctx.Storage.Listeners().Create(ctx, &storage.Listener{
		ID:             s.Hash,
		ProjectID:      projectID,
		Host:           s.Host,
		Port:           s.Port,
		PublicIP:       s.PublicIP,
		ShellPath:      s.ShellPath,
		DisableHistory: s.DisableHistory,
		GroupDispatch:  s.GroupDispatch,
		CreatedAt:      s.TimeStamp.UTC(),
	})
}

// PersistSessionForAgent writes the session row. Requires HostID +
// ProjectID to already be populated (UpsertHostForAgent does this) and
// the listener row to exist (EnsureListenerRow). Skips silently when
// storage is absent.
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
		ListenerID:     c.server.Hash,
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
	}
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
}
