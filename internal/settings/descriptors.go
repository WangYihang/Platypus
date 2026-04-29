package settings

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/WangYihang/Platypus/internal/storage"
)

// SettingDescriptor is the UI-facing projection of a single setting.
// It carries everything the Web UI needs to render the row: label,
// description, type (for input widget selection), current effective
// value, and whether that effective value came from the DB, YAML, or
// hardcoded default.
type SettingDescriptor struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	Section      string `json:"section"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	DefaultValue any    `json:"default"`
	YAMLValue    any    `json:"yaml,omitempty"`
	DBValue      any    `json:"db,omitempty"`
	Effective    any    `json:"effective"`
	Source       string `json:"source"` // "db" | "yaml" | "default"
}

// descriptorMeta is the static metadata for each registered setting.
// Dynamic values (Effective, DBValue, YAMLValue, DefaultValue) are
// computed at describe time by the registry.
type descriptorMeta struct {
	Key         string
	Type        string
	Section     string
	Label       string
	Description string
}

var allDescriptors = []descriptorMeta{
	{
		Key:         KeyAuthAccessTokenTTL,
		Type:        typeDurationSeconds,
		Section:     "auth",
		Label:       "Access token TTL",
		Description: "Lifetime of an issued access JWT, in seconds.",
	},
	{
		Key:         KeyAuthRefreshTokenTTL,
		Type:        typeDurationSeconds,
		Section:     "auth",
		Label:       "Refresh token TTL",
		Description: "Lifetime of an issued refresh JWT, in seconds.",
	},
	{
		Key:         KeyAuthPATDefaultTTL,
		Type:        typeDurationSeconds,
		Section:     "auth",
		Label:       "PAT default TTL",
		Description: "Default lifetime of a minted PAT when the admin doesn't specify one, in seconds.",
	},
	{
		Key:         KeyDistributorChannel,
		Type:        typeString,
		Section:     "distributor",
		Label:       "Default release channel",
		Description: "Channel shipped in agent install scripts (e.g. stable, beta, dev).",
	},
	{
		Key:         KeyDistributorPresignedTTL,
		Type:        typeDurationSeconds,
		Section:     "distributor",
		Label:       "Presigned URL TTL",
		Description: "How long S3 presigned download links stay valid, in seconds.",
	},
	{
		Key:         KeyMeshDiscoveryLAN,
		Type:        typeBool,
		Section:     "mesh",
		Label:       "LAN discovery (mDNS)",
		Description: "Broadcast mesh presence via mDNS for auto-discovery on the local network.",
	},
	{
		Key:         KeyMeshDiscoveryIntervalSec,
		Type:        typeDurationSeconds,
		Section:     "mesh",
		Label:       "Discovery interval",
		Description: "Seconds between mesh mDNS discovery cycles.",
	},
	{
		Key:         KeyAuditRetentionDays,
		Type:        typeInt,
		Section:     "audit",
		Label:       "Retention (days)",
		Description: "Delete audit entries older than this many days. 0 = keep forever.",
	},
	{
		Key:         KeyMeshPeers,
		Type:        typeStringList,
		Section:     "mesh",
		Label:       "Bootstrap peers",
		Description: "host:port addresses the mesh node dials at boot and on every reconcile tick. Live-editable; the Node picks up adds/removes on the next tick.",
	},
	{
		Key:         KeyEnrollmentRequireApproval,
		Type:        typeBool,
		Section:     "enrollment",
		Label:       "Require admin approval",
		Description: "When on, fresh agent enrollments land in `pending` and can't open a link until an admin approves them. PATs minted with auto_approve=true bypass this regardless of the global setting.",
	},
}

func descriptor(key string) (descriptorMeta, bool) {
	for _, m := range allDescriptors {
		if m.Key == key {
			return m, true
		}
	}
	return descriptorMeta{}, false
}

// DescribeAll returns a snapshot of every registered setting with its
// current effective value, YAML fallback, DB override (if any), and
// the hardcoded default. Never returns an error for a single key — if
// the DB row fails to parse, that key reports source="default" (or
// "yaml" if applicable).
func (r *Registry) DescribeAll(ctx context.Context) ([]SettingDescriptor, error) {
	out := make([]SettingDescriptor, 0, len(allDescriptors))
	for _, m := range allDescriptors {
		d := SettingDescriptor{
			Key:         m.Key,
			Type:        m.Type,
			Section:     m.Section,
			Label:       m.Label,
			Description: m.Description,
		}
		d.DefaultValue = r.describeDefault(m)
		d.YAMLValue = r.describeYAML(m)
		d.DBValue = r.describeDB(ctx, m)
		d.Effective = r.describeEffective(m)
		switch {
		case d.DBValue != nil:
			d.Source = "db"
		case d.YAMLValue != nil:
			d.Source = "yaml"
		default:
			d.Source = "default"
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *Registry) describeDefault(m descriptorMeta) any {
	switch m.Key {
	case KeyAuthAccessTokenTTL:
		return int64(DefaultAccessTokenTTL.Seconds())
	case KeyAuthRefreshTokenTTL:
		return int64(DefaultRefreshTokenTTL.Seconds())
	case KeyAuthPATDefaultTTL:
		return int64(DefaultPATDefaultTTL.Seconds())
	case KeyDistributorChannel:
		return DefaultDistributorChannel
	case KeyDistributorPresignedTTL:
		return int64(DefaultPresignedTTL.Seconds())
	case KeyMeshDiscoveryLAN:
		return DefaultMeshDiscoveryLAN
	case KeyMeshDiscoveryIntervalSec:
		return int64(DefaultMeshDiscoveryIntSecs)
	case KeyAuditRetentionDays:
		return int64(DefaultAuditRetentionDays)
	case KeyMeshPeers:
		return []string{}
	case KeyEnrollmentRequireApproval:
		return DefaultEnrollmentRequireApproval
	}
	return nil
}

// describeYAML returns the operator-supplied bootstrap value for
// m.Key, or nil when the bootstrap layer doesn't carry one.
//
// Phase 1 had a YAML config file with overrides for several runtime
// settings (channel, TTL, mesh discovery). Those YAML fields were
// retired alongside viper — the bootstrap layer is now flag/env-only
// and never carries runtime-policy values. This stub stays so the
// admin Settings UI's payload schema doesn't churn; it always returns
// nil now, which the UI renders as "default in code".
func (r *Registry) describeYAML(m descriptorMeta) any {
	return nil
}

// describeDB returns the parsed DB value for m.Key if an override
// exists, nil otherwise.
func (r *Registry) describeDB(ctx context.Context, m descriptorMeta) any {
	if r.db == nil {
		return nil
	}
	row, err := r.db.AdminSettings().Get(ctx, m.Key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return nil
	}
	switch m.Type {
	case typeDurationSeconds, typeInt:
		var n int64
		if json.Unmarshal([]byte(row.Value), &n) == nil {
			return n
		}
	case typeBool:
		var b bool
		if json.Unmarshal([]byte(row.Value), &b) == nil {
			return b
		}
	case typeString:
		var s string
		if json.Unmarshal([]byte(row.Value), &s) == nil {
			return s
		}
	case typeStringList:
		var l []string
		if json.Unmarshal([]byte(row.Value), &l) == nil {
			if l == nil {
				l = []string{}
			}
			return l
		}
	}
	return nil
}

func (r *Registry) describeEffective(m descriptorMeta) any {
	switch m.Key {
	case KeyAuthAccessTokenTTL:
		return int64(r.AccessTokenTTL().Seconds())
	case KeyAuthRefreshTokenTTL:
		return int64(r.RefreshTokenTTL().Seconds())
	case KeyAuthPATDefaultTTL:
		return int64(r.PATDefaultTTL().Seconds())
	case KeyDistributorChannel:
		return r.DistributorChannel()
	case KeyDistributorPresignedTTL:
		return int64(r.DistributorPresignedTTL().Seconds())
	case KeyMeshDiscoveryLAN:
		return r.MeshDiscoveryLAN()
	case KeyMeshDiscoveryIntervalSec:
		return int64(r.MeshDiscoveryInterval().Seconds())
	case KeyAuditRetentionDays:
		return int64(r.AuditRetentionDays())
	case KeyMeshPeers:
		return r.MeshPeers()
	case KeyEnrollmentRequireApproval:
		return r.EnrollmentRequireApproval()
	}
	return nil
}
