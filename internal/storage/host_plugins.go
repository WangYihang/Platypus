package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// HostPlugin is one row in host_plugins — the authoritative record of
// "this plugin, at this version, with this resolved config, is
// installed on this host". The composite primary key (host_id,
// plugin_id) gives upsert semantics: re-installing the same plugin on
// a host updates the row in place rather than appending.
//
// ConfigResolved is the post-secret-substitution form. Storing the
// resolved value (not the saved-with-refs form) is deliberate: if the
// underlying ProjectSecret is later revoked or rotated, the audit
// trail still answers "what config was actually deployed at install
// time". Re-installs after a rotation produce a new resolved config
// and update the row.
type HostPlugin struct {
	HostID              string
	PluginID            string
	Version             string
	GrantedCapabilities []agentplugin.CapabilityID
	// ConfigResolved is the JSON object the agent received. Stored
	// verbatim so the row faithfully represents what was deployed,
	// not what was saved — those can diverge after secret rotations.
	ConfigResolved json.RawMessage
	SchemaVersion  int
	State          HostPluginState
	InstalledAt    *time.Time
	UpdatedAt      time.Time
	LastError      string
}

// HostPluginState tracks the install lifecycle. Append-only as a
// matter of audit: a "removed" row is kept rather than deleted so
// the reconciler and the operator can answer "did host X ever run
// plugin Y?".
type HostPluginState string

const (
	HostPluginPending   HostPluginState = "pending"
	HostPluginInstalled HostPluginState = "installed"
	HostPluginFailed    HostPluginState = "failed"
	HostPluginRemoved   HostPluginState = "removed"
)

// HostPluginUpsert is the input shape for Upsert — it deliberately
// excludes UpdatedAt (the repo stamps it) and LastError handling is
// state-driven (only kept when state == failed; cleared otherwise).
type HostPluginUpsert struct {
	HostID              string
	PluginID            string
	Version             string
	GrantedCapabilities []agentplugin.CapabilityID
	ConfigResolved      json.RawMessage
	SchemaVersion       int
	State               HostPluginState
	LastError           string // honoured only when State == failed
}

func (db *DB) HostPlugins() *HostPluginRepo { return &HostPluginRepo{db: db.DB} }

type HostPluginRepo struct {
	db *sql.DB
}

// Upsert writes the row, replacing any existing (host_id, plugin_id)
// pair. installed_at is set on the first transition into "installed"
// and preserved across subsequent updates so the row's "first
// installed" timestamp survives version bumps and config tweaks.
func (r *HostPluginRepo) Upsert(ctx context.Context, in HostPluginUpsert) error {
	if in.HostID == "" || in.PluginID == "" {
		return errors.New("host_plugins: host_id + plugin_id required")
	}
	caps, err := encodeStringSlice(agentplugin.CapabilityIDsToStrings(in.GrantedCapabilities))
	if err != nil {
		return err
	}
	cfg := nullableJSON(in.ConfigResolved)

	// Compute the new state's installed_at. The CTE keeps the
	// timestamp stable across updates: if the row already has one,
	// reuse it; if it's transitioning into "installed" for the first
	// time, stamp now; otherwise leave NULL.
	now := time.Now().UTC()
	var installedAt interface{}
	if in.State == HostPluginInstalled {
		// Try to preserve the existing installed_at; fall through to
		// `now` if absent.
		var existing sql.NullTime
		err := r.db.QueryRowContext(ctx, `
			SELECT installed_at FROM host_plugins
			 WHERE host_id = ? AND plugin_id = ?`,
			in.HostID, in.PluginID,
		).Scan(&existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if existing.Valid {
			installedAt = existing.Time
		} else {
			installedAt = now
		}
	} else {
		installedAt = nil
	}

	lastError := nullableString("")
	if in.State == HostPluginFailed && in.LastError != "" {
		lastError = in.LastError
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO host_plugins (
			host_id, plugin_id, version, granted_capabilities,
			config_resolved, schema_version, state,
			installed_at, updated_at, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(host_id, plugin_id) DO UPDATE SET
			version              = excluded.version,
			granted_capabilities = excluded.granted_capabilities,
			config_resolved      = excluded.config_resolved,
			schema_version       = excluded.schema_version,
			state                = excluded.state,
			installed_at         = excluded.installed_at,
			updated_at           = excluded.updated_at,
			last_error           = excluded.last_error`,
		in.HostID, in.PluginID, in.Version, caps,
		cfg, in.SchemaVersion, string(in.State),
		installedAt, now, lastError,
	)
	return err
}

// Get fetches one row by (host_id, plugin_id). ErrNotFound on miss.
func (r *HostPluginRepo) Get(ctx context.Context, hostID, pluginID string) (*HostPlugin, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT host_id, plugin_id, version, granted_capabilities,
		       config_resolved, schema_version, state,
		       installed_at, updated_at, last_error
		  FROM host_plugins
		 WHERE host_id = ? AND plugin_id = ?`, hostID, pluginID)
	hp, err := scanHostPlugin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return hp, err
}

// ListByHost returns every plugin row for a host, including removed.
// Callers filter by State if they only want live plugins; keeping the
// full history visible lets the per-host UI render "previously
// installed" entries with their last config preserved.
func (r *HostPluginRepo) ListByHost(ctx context.Context, hostID string) ([]*HostPlugin, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT host_id, plugin_id, version, granted_capabilities,
		       config_resolved, schema_version, state,
		       installed_at, updated_at, last_error
		  FROM host_plugins
		 WHERE host_id = ?
		 ORDER BY plugin_id`, hostID)
	return collectHostPlugins(rows, err)
}

// ListHostsRunningPlugin returns the rows currently in `installed`
// state for the given plugin id. Useful for fleet-wide queries
// ("which hosts have datadog-forwarder live?") and the rollout
// reconciler. Optionally narrows by version — empty `version` means
// any.
func (r *HostPluginRepo) ListHostsRunningPlugin(ctx context.Context, pluginID, version string) ([]*HostPlugin, error) {
	if version == "" {
		rows, err := r.db.QueryContext(ctx, `
			SELECT host_id, plugin_id, version, granted_capabilities,
			       config_resolved, schema_version, state,
			       installed_at, updated_at, last_error
			  FROM host_plugins
			 WHERE plugin_id = ? AND state = 'installed'`, pluginID)
		return collectHostPlugins(rows, err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT host_id, plugin_id, version, granted_capabilities,
		       config_resolved, schema_version, state,
		       installed_at, updated_at, last_error
		  FROM host_plugins
		 WHERE plugin_id = ? AND version = ? AND state = 'installed'`, pluginID, version)
	return collectHostPlugins(rows, err)
}

// MarkRemoved transitions a row to the "removed" state without
// deleting it. Audit trail stays intact: the row's last config and
// version remain queryable. Returns ErrNotFound if no row exists.
func (r *HostPluginRepo) MarkRemoved(ctx context.Context, hostID, pluginID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE host_plugins
		   SET state = 'removed', updated_at = ?, last_error = NULL
		 WHERE host_id = ? AND plugin_id = ?`,
		time.Now().UTC(), hostID, pluginID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func collectHostPlugins(rows *sql.Rows, err error) ([]*HostPlugin, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*HostPlugin
	for rows.Next() {
		hp, err := scanHostPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, hp)
	}
	return out, rows.Err()
}

func scanHostPlugin(row rowScanner) (*HostPlugin, error) {
	var (
		hp          HostPlugin
		caps        sql.NullString
		cfg         sql.NullString
		state       string
		installedAt sql.NullTime
		lastErr     sql.NullString
	)
	err := row.Scan(
		&hp.HostID, &hp.PluginID, &hp.Version, &caps,
		&cfg, &hp.SchemaVersion, &state,
		&installedAt, &hp.UpdatedAt, &lastErr,
	)
	if err != nil {
		return nil, err
	}
	if caps.Valid && caps.String != "" {
		ids, err := decodeStringSlice(caps.String)
		if err != nil {
			return nil, fmt.Errorf("host_plugins: decode caps: %w", err)
		}
		hp.GrantedCapabilities = agentplugin.CapabilityIDsFromStrings(ids)
	}
	if cfg.Valid && cfg.String != "" {
		hp.ConfigResolved = json.RawMessage(cfg.String)
	}
	hp.State = HostPluginState(state)
	if installedAt.Valid {
		t := installedAt.Time
		hp.InstalledAt = &t
	}
	hp.LastError = lastErr.String
	return &hp, nil
}

// encodeStringSlice marshals an []string for storage in a TEXT
// column. Empty / nil → NULL so the absence shows up at the SQL
// level. Used by host_plugins.granted_capabilities and any future
// caller that needs the same shape.
func encodeStringSlice(s []string) (interface{}, error) {
	if len(s) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func decodeStringSlice(s string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// nullableJSON maps a json.RawMessage to a SQL value: empty / nil →
// NULL, otherwise the raw bytes as a string. Mirrors nullableString
// for the "this column holds JSON or nothing" case.
func nullableJSON(r json.RawMessage) interface{} {
	if len(r) == 0 {
		return nil
	}
	return string(r)
}
