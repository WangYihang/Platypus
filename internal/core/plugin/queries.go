package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// PluginRow is the SELECT-friendly shape returned by Search / Get.
// Differs from IndexEntry only in that capabilities + tags are
// already-decoded slices instead of raw JSON strings.
type PluginRow struct {
	PluginID       string   `json:"plugin_id"`
	Version        string   `json:"version"`
	Name           string   `json:"name"`
	Author         string   `json:"author"`
	License        string   `json:"license"`
	Homepage       string   `json:"homepage"`
	Description    string   `json:"description"`
	LatestVersion  string   `json:"latest_version"`
	PublisherKeyID string   `json:"publisher_key_id"`
	WasmURL        string   `json:"wasm_url"`
	SignatureURL   string   `json:"signature_url"`
	ManifestURL    string   `json:"manifest_url,omitempty"`
	WasmSHA256Hex  string   `json:"wasm_sha256_hex"`
	Capabilities   []string `json:"capabilities"`
	Tags           []string `json:"tags,omitempty"`
	FetchedAtUnix  int64    `json:"fetched_at_unix"`

	// PublisherPubkey is the raw .pub file contents (minisign Ed25519
	// public key) the agent uses to verify the wasm signature on
	// install. Empty for legacy index files; the install_marketplace
	// REST endpoint refuses to install when empty so the operator
	// hits a clear error instead of a silent agent-side rejection.
	// JSON-omitted: never wanted on the wire — REST clients shouldn't
	// touch the raw key, the install endpoint reads it directly from
	// the catalog.
	PublisherPubkey []byte `json:"-"`
}

// Search returns the latest version of every plugin whose name
// contains q (case-insensitive). q="" returns everything. Result is
// sorted by name for deterministic UI rendering.
func (c *Catalog) Search(ctx context.Context, q string) ([]PluginRow, error) {
	const baseQuery = `
		SELECT plugin_id, version, name, author, license, homepage,
		       description, latest_version, publisher_key_id,
		       wasm_url, signature_url, manifest_url, wasm_sha256_hex,
		       capabilities_json, tags_json, fetched_at_unix,
		       publisher_pubkey
		  FROM marketplace_plugin_versions
		 WHERE version = latest_version`

	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = c.db.QueryContext(ctx, baseQuery+` ORDER BY name`)
	} else {
		// LIKE on lowercase name. SQLite's LIKE is case-insensitive
		// for ASCII by default; for unicode-correctness this would
		// want LOWER(name) LIKE LOWER(?), but the manifests' name
		// fields are ASCII in practice.
		rows, err = c.db.QueryContext(ctx,
			baseQuery+` AND name LIKE ? ORDER BY name`, "%"+q+"%")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows)
}

// Versions returns every cached version of one plugin id, sorted
// newest-first (lexical-DESC on the version string — fine for
// strict semver MAJOR.MINOR.PATCH).
func (c *Catalog) Versions(ctx context.Context, pluginID string) ([]PluginRow, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT plugin_id, version, name, author, license, homepage,
		       description, latest_version, publisher_key_id,
		       wasm_url, signature_url, manifest_url, wasm_sha256_hex,
		       capabilities_json, tags_json, fetched_at_unix,
		       publisher_pubkey
		  FROM marketplace_plugin_versions
		 WHERE plugin_id = ?
		 ORDER BY version DESC`, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows)
}

// Get returns one specific (plugin_id, version). The (false, nil)
// "no such row" case is the common one for an operator pasting a
// stale link.
func (c *Catalog) Get(ctx context.Context, pluginID, version string) (PluginRow, bool, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT plugin_id, version, name, author, license, homepage,
		       description, latest_version, publisher_key_id,
		       wasm_url, signature_url, manifest_url, wasm_sha256_hex,
		       capabilities_json, tags_json, fetched_at_unix,
		       publisher_pubkey
		  FROM marketplace_plugin_versions
		 WHERE plugin_id = ? AND version = ?`, pluginID, version)
	r, err := scanRow(row.Scan)
	if err == sql.ErrNoRows {
		return PluginRow{}, false, nil
	}
	if err != nil {
		return PluginRow{}, false, err
	}
	return r, true, nil
}

// LastRefresh is the single-row "when did the catalog sync last?"
// readout the operator UI shows in the Marketplace tab header.
type RefreshStatus struct {
	IndexURL        string `json:"index_url"`
	LastFetchedUnix int64  `json:"last_fetched_unix"`
	LastStatus      string `json:"last_status"`
	LastError       string `json:"last_error,omitempty"`
	PluginCount     int    `json:"plugin_count"`
}

func (c *Catalog) LastRefresh(ctx context.Context) (RefreshStatus, bool, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT index_url, last_fetched_unix, last_status, last_error, plugin_count
		  FROM marketplace_index_refreshes
		 ORDER BY last_fetched_unix DESC
		 LIMIT 1`)
	var rs RefreshStatus
	err := row.Scan(&rs.IndexURL, &rs.LastFetchedUnix, &rs.LastStatus, &rs.LastError, &rs.PluginCount)
	if err == sql.ErrNoRows {
		return RefreshStatus{}, false, nil
	}
	if err != nil {
		return RefreshStatus{}, false, err
	}
	return rs, true, nil
}

// scanRow / collectRows centralise the SELECT field mapping +
// capabilities/tags JSON unmarshal so each per-query function
// stays a few lines.
func scanRow(scan func(...any) error) (PluginRow, error) {
	var (
		r        PluginRow
		capsJSON string
		tagsJSON string
	)
	if err := scan(
		&r.PluginID, &r.Version, &r.Name, &r.Author, &r.License, &r.Homepage,
		&r.Description, &r.LatestVersion, &r.PublisherKeyID,
		&r.WasmURL, &r.SignatureURL, &r.ManifestURL, &r.WasmSHA256Hex,
		&capsJSON, &tagsJSON, &r.FetchedAtUnix,
		&r.PublisherPubkey,
	); err != nil {
		return PluginRow{}, err
	}
	if strings.TrimSpace(capsJSON) != "" {
		_ = json.Unmarshal([]byte(capsJSON), &r.Capabilities)
	}
	if strings.TrimSpace(tagsJSON) != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &r.Tags)
	}
	return r, nil
}

func collectRows(rows *sql.Rows) ([]PluginRow, error) {
	var out []PluginRow
	for rows.Next() {
		r, err := scanRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
