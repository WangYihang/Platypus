-- 000029_marketplace_plugins.sql — server-side cache of the
-- platypus-plugins git index repo. The cache is populated by
-- internal/core/plugin/catalog.go (periodic refresh from the index
-- URL) and consulted by the marketplace REST endpoints + the
-- per-agent install handler when an operator picks "install latest"
-- without specifying inline bytes.
--
-- One row per (plugin_id, version). Plugin metadata that doesn't
-- vary per version (display name, author, license, homepage, latest
-- version pointer) is denormalised onto every row of that plugin so
-- list queries can `SELECT … WHERE version = latest_version` without
-- a join. Tradeoff: a metadata field rename would touch every
-- version row instead of one — but the catalog is small (hundreds
-- to low thousands of plugins) and fully rebuilt on every refresh,
-- so the write cost is negligible.

CREATE TABLE marketplace_plugin_versions (
    plugin_id           TEXT NOT NULL,
    version             TEXT NOT NULL,                    -- semver; PK with plugin_id
    name                TEXT NOT NULL,
    author              TEXT NOT NULL DEFAULT '',
    license             TEXT NOT NULL DEFAULT '',
    homepage            TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    latest_version      TEXT NOT NULL,                    -- denormalised "latest" pointer
    publisher_key_id    TEXT NOT NULL,
    -- Where the agent fetches the artefacts from on install. Two
    -- of these (wasm + signature) are mandatory for URL-source
    -- installs; manifest URL is optional (the agent can also be
    -- handed the manifest inline via the REST install body).
    wasm_url            TEXT NOT NULL,
    signature_url       TEXT NOT NULL,
    manifest_url        TEXT NOT NULL DEFAULT '',
    wasm_sha256_hex     TEXT NOT NULL,                    -- 64 hex chars, lowercase
    -- Capabilities the manifest declares (NOT what an operator has
    -- granted — that's per-install). Stored as a JSON array of
    -- capability ids ("fs.read", "exec", …). The marketplace UI
    -- renders this verbatim in the Install dialog so an operator
    -- can scrutinise before clicking through.
    capabilities_json   TEXT NOT NULL DEFAULT '[]',
    -- Free-form keywords the index repo's CI extracts from the
    -- manifest's description / category fields. Powers fuzzy search.
    tags_json           TEXT NOT NULL DEFAULT '[]',
    -- When this row was last refreshed from the index. The catalog
    -- worker compares against now() to decide if a refresh is
    -- overdue.
    fetched_at_unix     INTEGER NOT NULL,
    PRIMARY KEY (plugin_id, version)
);

-- Search by id is the dominant query path (agent install flow knows
-- the id). The "latest version" lookup is satisfied by
-- WHERE version = latest_version which uses the PK.
CREATE INDEX idx_marketplace_plugin_versions_latest
    ON marketplace_plugin_versions(plugin_id, latest_version);

-- Sort-by-name browsing of the marketplace.
CREATE INDEX idx_marketplace_plugin_versions_name
    ON marketplace_plugin_versions(name);

-- Tracks the index refresh as a whole — not per-row — so the UI can
-- show "last marketplace sync at HH:MM" without an aggregate query.
-- One row per index URL the server has ever fetched (operator
-- typically configures one, but multi-tenant deployments may federate).
CREATE TABLE marketplace_index_refreshes (
    index_url           TEXT PRIMARY KEY,
    last_fetched_unix   INTEGER NOT NULL,
    last_status         TEXT NOT NULL,                    -- 'ok' | 'http_error' | 'parse_error' | 'signature_error'
    last_error          TEXT NOT NULL DEFAULT '',
    plugin_count        INTEGER NOT NULL DEFAULT 0
);
