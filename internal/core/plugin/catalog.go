// Package plugin (server-side) caches the marketplace index repo
// (com.platypus.platypus-plugins-style git tree) into SQLite so the
// operator UI can browse / search without round-tripping to GitHub
// per click, and so the per-agent install flow can fetch the
// matching wasm + signature URLs deterministically.
//
// The cache is one-direction: the index repo is the source of truth,
// the catalog is a denormalised SELECT-friendly mirror. Refreshes
// happen on a periodic worker (RefreshLoop) plus on explicit
// operator-triggered refresh via the REST endpoint.
//
// The "index format" the catalog expects is the JSON file the index
// repo's CI generates from the per-plugin manifests; see
// docs/plugins/AUTHORS.md (Marketplace section) for the contract.
// Format is simple enough that the server doesn't need a
// platypus-plugins SDK — encoding/json + this struct set is
// sufficient.
package plugin

import (
	"context"
	"database/sql"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// base64Std is a tiny alias kept readable at the call site —
// `base64Std.DecodeString` reads cleaner than spelling out
// `base64.StdEncoding.DecodeString` inline in the Refresh loop.
var base64Std = b64.StdEncoding

// IndexEntry is the per-version row the index.json publishes. One
// plugin contributes one entry per published version.
type IndexEntry struct {
	PluginID         string   `json:"plugin_id"`
	Version          string   `json:"version"`
	Name             string   `json:"name"`
	Author           string   `json:"author,omitempty"`
	License          string   `json:"license,omitempty"`
	Homepage         string   `json:"homepage,omitempty"`
	Description      string   `json:"description,omitempty"`
	LatestVersion    string   `json:"latest_version"`
	PublisherKeyID   string   `json:"publisher_key_id"`
	WasmURL          string   `json:"wasm_url"`
	SignatureURL     string   `json:"signature_url"`
	ManifestURL      string   `json:"manifest_url,omitempty"`
	WasmSHA256Hex    string   `json:"wasm_sha256_hex"`
	Capabilities     []string `json:"capabilities"`
	Tags             []string `json:"tags,omitempty"`

	// PublisherPubkeyB64 is the base64 of the publisher's minisign
	// .pub file. Optional in the index format for backward compat;
	// when absent the install_marketplace REST endpoint refuses to
	// install (the agent can't verify the signature without it).
	PublisherPubkeyB64 string `json:"publisher_pubkey_b64,omitempty"`
}

// Index is the top-level shape of index.json.
type Index struct {
	GeneratedAt int64        `json:"generated_at_unix"`
	Plugins     []IndexEntry `json:"plugins"`
}

// Catalog wraps the SQLite tables. Constructed once at server
// startup with the live *sql.DB from internal/storage. Methods are
// goroutine-safe (sqlite handles its own locking).
type Catalog struct {
	db        *sql.DB
	indexURL  string
	httpClient *http.Client
	now        func() time.Time
}

// New constructs a Catalog. indexURL points at the public index.json;
// empty means "no index configured" — Refresh becomes a no-op + the
// REST search endpoint returns whatever's already cached (possibly
// nothing on a fresh install).
func New(db *sql.DB, indexURL string) *Catalog {
	return &Catalog{
		db:         db,
		indexURL:   indexURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

// Refresh fetches indexURL and replaces every cached row. The whole
// refresh runs in a single transaction so a partial failure leaves
// the previous catalog intact. Returns the number of plugin-version
// rows landed.
func (c *Catalog) Refresh(ctx context.Context) (int, error) {
	if c.indexURL == "" {
		return 0, nil
	}
	idx, err := c.fetchIndex(ctx)
	status := "ok"
	errMsg := ""
	if err != nil {
		status = errorBucket(err)
		errMsg = err.Error()
		_ = c.recordRefresh(ctx, status, errMsg, 0)
		return 0, err
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		_ = c.recordRefresh(ctx, "db_error", err.Error(), 0)
		return 0, fmt.Errorf("plugin catalog: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM marketplace_plugin_versions`); err != nil {
		_ = c.recordRefresh(ctx, "db_error", err.Error(), 0)
		return 0, fmt.Errorf("plugin catalog: clear: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		_ = c.recordRefresh(ctx, "db_error", err.Error(), 0)
		return 0, fmt.Errorf("plugin catalog: prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range idx.Plugins {
		caps, _ := json.Marshal(e.Capabilities)
		tags, _ := json.Marshal(e.Tags)
		// Publisher pubkey is base64-encoded in the index for JSON
		// transport; we decode here so SQLite stores raw bytes.
		// Empty/invalid → empty BLOB (NOT nil — SQLite rejects nil
		// against the NOT NULL column even with a DEFAULT). Legacy
		// index files keep working: install_marketplace will surface
		// the empty key as 424 Failed Dependency rather than fail at
		// insert.
		pubkey := []byte{}
		if e.PublisherPubkeyB64 != "" {
			if decoded, derr := base64Std.DecodeString(e.PublisherPubkeyB64); derr == nil {
				pubkey = decoded
			}
		}
		if _, err := stmt.ExecContext(ctx,
			e.PluginID, e.Version, e.Name, e.Author, e.License, e.Homepage,
			e.Description, e.LatestVersion, e.PublisherKeyID,
			e.WasmURL, e.SignatureURL, e.ManifestURL, e.WasmSHA256Hex,
			string(caps), string(tags), c.now().Unix(),
			pubkey,
		); err != nil {
			_ = c.recordRefresh(ctx, "db_error", err.Error(), 0)
			return 0, fmt.Errorf("plugin catalog: insert %s/%s: %w",
				e.PluginID, e.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("plugin catalog: commit: %w", err)
	}
	if err := c.recordRefresh(ctx, status, errMsg, len(idx.Plugins)); err != nil {
		return len(idx.Plugins), fmt.Errorf("plugin catalog: record refresh: %w", err)
	}
	return len(idx.Plugins), nil
}

const insertSQL = `INSERT INTO marketplace_plugin_versions
    (plugin_id, version, name, author, license, homepage, description,
     latest_version, publisher_key_id, wasm_url, signature_url,
     manifest_url, wasm_sha256_hex, capabilities_json, tags_json,
     fetched_at_unix, publisher_pubkey)
    VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

// fetchIndex reads + parses the JSON. Separated so tests can
// substitute the http.Client. Supports both http(s):// and file://
// URLs — the latter is how the dev-mode publisher container hands the
// server an index without standing up a separate HTTP server inside
// the compose stack.
func (c *Catalog) fetchIndex(ctx context.Context) (*Index, error) {
	body, err := readURLBytes(ctx, c.httpClient, c.indexURL)
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("index parse: %w", err)
	}
	return &idx, nil
}

// readURLBytes is the shared "fetch a URL into a byte slice" helper
// used by the catalog refresh + (via the api package) the install-from-
// marketplace artefact fetcher. file:// URLs are handled by reading
// directly from disk; everything else goes through the http.Client.
//
// Limited to what the refresh path needs today — no streaming, no
// content-type sniffing. Caller decides what to do with the bytes.
func readURLBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if strings.HasPrefix(url, "file://") {
		path := strings.TrimPrefix(url, "file://")
		// Reject `..` traversal so a malicious index.json can't ask
		// the server to read /etc/passwd at install time. The dev
		// publisher only ever writes under <data-dir>, so the cleaned
		// path should be absolute and free of relative segments.
		if strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") {
			return nil, fmt.Errorf("file url path contains parent traversal: %s", path)
		}
		return readFileBounded(path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// readFileBounded reads a local file, capped at maxLocalArtefactBytes.
// Cap matches the http fetch's implicit ceiling (LimitReader at 64 MiB
// in api.NewHTTPArtefactFetcher) so the failure surface is the same
// regardless of whether the index points at HTTP or file://.
func readFileBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxLocalArtefactBytes+1))
}

const maxLocalArtefactBytes = 64 << 20 // 64 MiB

// errorBucket maps low-level errors to the small set of buckets the
// refresh row records. Coarse on purpose — the UI cares about
// "what kind of failure?" not "which line of which file?".
func errorBucket(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "HTTP "):
		return "http_error"
	case strings.Contains(msg, "index parse"):
		return "parse_error"
	default:
		return "fetch_error"
	}
}

// recordRefresh upserts the one-row-per-index status. Best-effort:
// logged errors bubble up to the caller but Refresh has already
// committed (or rolled back) the bulk catalog write before this
// runs, so a recordRefresh failure doesn't corrupt state.
func (c *Catalog) recordRefresh(ctx context.Context, status, errMsg string, count int) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO marketplace_index_refreshes
		    (index_url, last_fetched_unix, last_status, last_error, plugin_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(index_url) DO UPDATE SET
		    last_fetched_unix = excluded.last_fetched_unix,
		    last_status       = excluded.last_status,
		    last_error        = excluded.last_error,
		    plugin_count      = excluded.plugin_count`,
		c.indexURL, c.now().Unix(), status, errMsg, count)
	return err
}
