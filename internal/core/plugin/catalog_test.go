package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Apply just the marketplace migration. Keeps the test
	// self-contained; the production startup path runs every
	// migration via internal/storage's runMigrations.
	if _, err := db.ExecContext(context.Background(), marketplaceSchema); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

const marketplaceSchema = `
CREATE TABLE marketplace_plugin_versions (
    plugin_id           TEXT NOT NULL,
    version             TEXT NOT NULL,
    name                TEXT NOT NULL,
    author              TEXT NOT NULL DEFAULT '',
    license             TEXT NOT NULL DEFAULT '',
    homepage            TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    latest_version      TEXT NOT NULL,
    publisher_key_id    TEXT NOT NULL,
    wasm_url            TEXT NOT NULL,
    signature_url       TEXT NOT NULL,
    manifest_url        TEXT NOT NULL DEFAULT '',
    wasm_sha256_hex     TEXT NOT NULL,
    capabilities_json   TEXT NOT NULL DEFAULT '[]',
    tags_json           TEXT NOT NULL DEFAULT '[]',
    fetched_at_unix     INTEGER NOT NULL,
    publisher_pubkey    BLOB NOT NULL DEFAULT x'',
    PRIMARY KEY (plugin_id, version)
);
CREATE TABLE marketplace_index_refreshes (
    index_url           TEXT PRIMARY KEY,
    last_fetched_unix   INTEGER NOT NULL,
    last_status         TEXT NOT NULL,
    last_error          TEXT NOT NULL DEFAULT '',
    plugin_count        INTEGER NOT NULL DEFAULT 0
);`

func TestCatalog_RefreshPopulatesAndSearchFinds(t *testing.T) {
	db := newTestDB(t)

	idx := Index{
		GeneratedAt: 1700000000,
		Plugins: []IndexEntry{
			{PluginID: "com.example.alpha", Version: "1.0.0", Name: "Alpha",
				LatestVersion: "1.0.0", PublisherKeyID: "AAA",
				WasmURL: "https://example/a.wasm", SignatureURL: "https://example/a.sig",
				WasmSHA256Hex: "deadbeef", Capabilities: []string{"fs.read"},
			},
			{PluginID: "com.example.beta", Version: "2.0.0", Name: "Beta Tool",
				LatestVersion: "2.0.0", PublisherKeyID: "BBB",
				WasmURL: "https://example/b.wasm", SignatureURL: "https://example/b.sig",
				WasmSHA256Hex: "cafebabe", Capabilities: []string{"exec"},
				Tags: []string{"network", "diagnostics"},
			},
			// Older version of beta — should not appear in default Search.
			{PluginID: "com.example.beta", Version: "1.5.0", Name: "Beta Tool",
				LatestVersion: "2.0.0", PublisherKeyID: "BBB",
				WasmURL: "https://example/b-1.5.wasm", SignatureURL: "https://example/b-1.5.sig",
				WasmSHA256Hex: "cafebabe", Capabilities: []string{"exec"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(idx)
	}))
	defer srv.Close()

	cat := New(db, srv.URL)
	cat.now = func() time.Time { return time.Unix(1700000100, 0) }

	n, err := cat.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if n != 3 {
		t.Errorf("inserted = %d, want 3", n)
	}

	// Search returns 2 results (latest of each plugin id).
	rows, err := cat.Search(context.Background(), "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("search empty = %d rows, want 2", len(rows))
	}
	// Sorted by name: "Alpha" before "Beta Tool".
	if rows[0].PluginID != "com.example.alpha" {
		t.Errorf("first row = %s, want alpha", rows[0].PluginID)
	}

	// Filtered search.
	rows, _ = cat.Search(context.Background(), "Beta")
	if len(rows) != 1 || rows[0].PluginID != "com.example.beta" {
		t.Errorf("filtered = %+v", rows)
	}
	if len(rows[0].Capabilities) != 1 || rows[0].Capabilities[0] != "exec" {
		t.Errorf("capabilities decode = %+v", rows[0].Capabilities)
	}
	if len(rows[0].Tags) != 2 {
		t.Errorf("tags decode = %+v", rows[0].Tags)
	}

	// Versions returns both rows for beta, newest-first.
	versions, _ := cat.Versions(context.Background(), "com.example.beta")
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	if versions[0].Version != "2.0.0" || versions[1].Version != "1.5.0" {
		t.Errorf("versions order = %+v", versions)
	}

	// Get specific version.
	got, ok, err := cat.Get(context.Background(), "com.example.alpha", "1.0.0")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.Name != "Alpha" {
		t.Errorf("name = %q", got.Name)
	}

	// Refresh status row exists.
	rs, ok, err := cat.LastRefresh(context.Background())
	if err != nil || !ok {
		t.Fatalf("last refresh: %v / %v", err, ok)
	}
	if rs.PluginCount != 3 || rs.LastStatus != "ok" {
		t.Errorf("refresh row = %+v", rs)
	}
}

func TestCatalog_RefreshHTTPErrorRecorded(t *testing.T) {
	db := newTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cat := New(db, srv.URL)
	cat.now = func() time.Time { return time.Unix(1700000200, 0) }

	if _, err := cat.Refresh(context.Background()); err == nil {
		t.Fatalf("expected refresh error")
	}
	rs, ok, err := cat.LastRefresh(context.Background())
	if err != nil || !ok {
		t.Fatalf("last refresh: %v / %v", err, ok)
	}
	if rs.LastStatus != "http_error" {
		t.Errorf("status = %q, want http_error", rs.LastStatus)
	}
}

func TestCatalog_EmptyIndexURLNoOps(t *testing.T) {
	db := newTestDB(t)
	cat := New(db, "")
	n, err := cat.Refresh(context.Background())
	if err != nil || n != 0 {
		t.Errorf("empty index URL: n=%d err=%v", n, err)
	}
}

// TestCatalog_RefreshLoop_TicksAndExits verifies the periodic refresh
// worker:
//   - calls Refresh on first tick (kick-start, not interval-delayed)
//   - calls Refresh again every `interval`
//   - exits cleanly when its ctx is cancelled
//
// The test uses a tight interval (5 ms) and waits for at least 3
// successful hits — well within the test's overall 1 s budget.
func TestCatalog_RefreshLoop_TicksAndExits(t *testing.T) {
	db := newTestDB(t)

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(Index{GeneratedAt: 1, Plugins: nil})
	}))
	defer srv.Close()

	cat := New(db, srv.URL)
	cat.now = func() time.Time { return time.Unix(1700000200, 0) }

	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() {
		cat.RefreshLoop(ctx, 5*time.Millisecond)
		close(loopDone)
	}()

	// Wait for the loop to land at least 3 hits (initial + 2 ticks).
	deadline := time.Now().Add(time.Second)
	for hits.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := hits.Load(); got < 3 {
		cancel()
		<-loopDone
		t.Fatalf("hits = %d, want >= 3 within 1 s", got)
	}

	cancel()
	select {
	case <-loopDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RefreshLoop did not exit after ctx cancel")
	}
}

// TestCatalog_RefreshLoop_EmptyURLReturnsImmediately: with no index
// URL the worker should not spin a goroutine forever — it returns
// immediately so callers can spawn it unconditionally.
func TestCatalog_RefreshLoop_EmptyURLReturnsImmediately(t *testing.T) {
	db := newTestDB(t)
	cat := New(db, "")
	done := make(chan struct{})
	go func() {
		cat.RefreshLoop(context.Background(), 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("RefreshLoop with empty URL should return immediately")
	}
}

// TestCatalog_RefreshLoop_TolerantOfTransientError: the worker must
// keep ticking even when an upstream refresh fails (HTTP 5xx, parse
// error, etc.). We start with a server that always 500s, observe
// multiple failed attempts, then cancel.
func TestCatalog_RefreshLoop_TolerantOfTransientError(t *testing.T) {
	db := newTestDB(t)

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cat := New(db, srv.URL)
	cat.now = func() time.Time { return time.Unix(1700000200, 0) }

	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() {
		cat.RefreshLoop(ctx, 5*time.Millisecond)
		close(loopDone)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for hits.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-loopDone

	if got := hits.Load(); got < 3 {
		t.Errorf("expected >= 3 hits despite 500s, got %d", got)
	}
}
