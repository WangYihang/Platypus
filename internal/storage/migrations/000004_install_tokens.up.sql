-- 000004_install_tokens.up.sql — one-shot download tokens that gate the
-- `curl https://distributor/install/<id> | sh` bootstrap flow. Each
-- token, on first successful GET, atomically mints a fresh PAT and
-- embeds it in the rendered shell script. The PAT never exists on the
-- server before the token is consumed, so an unused token has zero
-- blast radius if it leaks.
--
-- Like the PAT tables, this is append-only: consumed / expired /
-- revoked rows remain forever and status is derived at read time.

CREATE TABLE install_download_tokens (
    download_id            TEXT PRIMARY KEY,              -- "dl_XXXX"
    secret_hash            BLOB NOT NULL,
    project_id             TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    issued_by_user         TEXT NOT NULL REFERENCES users(id),
    issued_at              DATETIME NOT NULL,
    expires_at             DATETIME NOT NULL,             -- default now + 300s
    -- Deployment targeting — the distributor builds (or fetches) a
    -- binary for this pair; empty means "use the first available build".
    target_os              TEXT,
    target_arch            TEXT,
    -- Server endpoint the bootstrap script hardcodes into the agent
    -- command line ("--host X --port Y"). Without it the script is useless.
    server_endpoint        TEXT NOT NULL,
    -- Provisioning preferences carried through to the PAT that gets
    -- minted on consumption. pat_binding_machine_id mirrors the PAT
    -- column; when non-empty it forces the bootstrapping machine to
    -- match a pre-committed /etc/machine-id value.
    pat_ttl_seconds        INTEGER NOT NULL DEFAULT 3600,
    pat_binding_machine_id TEXT,
    pat_description        TEXT,
    -- When first successful consume happens we mark these. The
    -- consumed_pat_id lets operators trace "who actually used this
    -- download link" across the two tables.
    consumed_at            DATETIME,
    consumed_ip            TEXT,
    consumed_ua            TEXT,
    consumed_pat_id        TEXT REFERENCES pat_tokens(token_id),
    revoked                INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
    revoked_at             DATETIME,
    revoked_by_user        TEXT REFERENCES users(id),
    revoked_reason         TEXT,
    CHECK (pat_ttl_seconds > 0),
    CHECK (revoked = 0 OR revoked_at IS NOT NULL)
    -- consumed_ip is captured when available but tolerated NULL: in-process
    -- tests use sockets without a meaningful RemoteAddr, and production
    -- clients behind certain proxies may not expose one either. The audit
    -- trail in install_download_events still records who / when.
);
-- Partial index: fast lookup of live (minted but not yet used) tokens.
-- Deterministic predicate only (revoked=0 and consumed_at IS NULL).
CREATE INDEX idx_install_dl_live
    ON install_download_tokens(project_id, expires_at)
    WHERE revoked = 0 AND consumed_at IS NULL;

-- Per-attempt audit log. Captures both success ("this token actually
-- redeemed") and failure modes so unauthorised curl attempts (wrong
-- secret, expired, already consumed) show up in security dashboards.
-- No FK to install_download_tokens: same rationale as pat_redemption_events —
-- we want to record attempts against nonexistent ids too.
CREATE TABLE install_download_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    at            DATETIME NOT NULL,
    download_id   TEXT NOT NULL,
    client_ip     TEXT,
    client_ua     TEXT,
    pat_token_id  TEXT,                -- set on success to tie event → minted PAT
    outcome       TEXT NOT NULL,
    error_detail  TEXT,
    CHECK (outcome IN (
        'success',
        'unknown_id',
        'invalid_secret',
        'expired',
        'revoked',
        'already_consumed',
        'malformed'
    ))
);
CREATE INDEX idx_install_dl_events_dl ON install_download_events(download_id, at);
CREATE INDEX idx_install_dl_events_ip ON install_download_events(client_ip, at);
CREATE INDEX idx_install_dl_events_at ON install_download_events(at);
