-- 000017_user_pat.up.sql — rebuild auth_tokens to (a) drop 'aat' from
-- the CHECK kind enum and (b) add the new 'pat' kind for user-issued
-- personal access tokens.
--
-- SQLite cannot ALTER a CHECK constraint in place, so the standard
-- table-rebuild dance applies: create the new shape, copy rows over,
-- drop the original, rename. Because migration 16 already deleted
-- every kind='aat' row, the INSERT...SELECT moves only user_session
-- rows (and any pre-existing PAT rows, of which there should be none
-- on a freshly-migrated DB).
--
-- Schema rules enforced by the new CHECK:
--
--   • kind = 'pat'           — user-bound long-lived API token. Carries
--     a name + scopes; never bound to a project (project access is
--     re-evaluated against project_members per request) and never
--     overrides the user's role (the verifier reads the live
--     users.role and intersects scopes against it).
--   • kind = 'user_session'  — sliding-window browser session,
--     unchanged from migration 13.

CREATE TABLE auth_tokens_new (
    token_id          TEXT PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('pat', 'user_session')),
    secret_hash       BLOB NOT NULL,
    user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT,
    description       TEXT,
    created_at        DATETIME NOT NULL,
    expires_at        DATETIME NOT NULL,
    last_used_at      DATETIME,
    last_used_ip      TEXT,
    user_agent        TEXT,
    revoked_at        DATETIME,
    revoked_by_user   TEXT REFERENCES users(id),
    revoked_reason    TEXT,
    project_id        TEXT REFERENCES projects(id) ON DELETE CASCADE,
    role              TEXT CHECK (role IS NULL OR role IN ('admin','operator','viewer')),
    scopes            TEXT,
    idle_expires_at   DATETIME,
    CHECK (
        (kind = 'pat'
            AND name IS NOT NULL
            AND scopes IS NOT NULL
            AND project_id IS NULL
            AND role IS NULL
            AND idle_expires_at IS NULL)
     OR (kind = 'user_session'
            AND project_id IS NULL
            AND role IS NULL
            AND scopes IS NULL
            AND idle_expires_at IS NOT NULL)
    )
);

INSERT INTO auth_tokens_new (
    token_id, kind, secret_hash, user_id,
    name, description,
    created_at, expires_at,
    last_used_at, last_used_ip, user_agent,
    revoked_at, revoked_by_user, revoked_reason,
    project_id, role, scopes,
    idle_expires_at
)
SELECT
    token_id, kind, secret_hash, user_id,
    name, description,
    created_at, expires_at,
    last_used_at, last_used_ip, user_agent,
    revoked_at, revoked_by_user, revoked_reason,
    project_id, role, scopes,
    idle_expires_at
FROM auth_tokens;

DROP TABLE auth_tokens;
ALTER TABLE auth_tokens_new RENAME TO auth_tokens;

-- Listing tokens for a user (settings UI / admin audit). DESC on
-- created_at so the newest entries come first without an in-app sort.
CREATE INDEX idx_auth_tokens_user_kind ON auth_tokens (user_id, kind, created_at DESC);

-- Per-user active PAT enumeration, partial so the index covers only
-- live PAT rows.
CREATE INDEX idx_auth_tokens_pat_active ON auth_tokens (user_id, expires_at)
    WHERE kind = 'pat' AND revoked_at IS NULL;

-- Active session enumeration / GC for a user. Same partial-index
-- trick keeps the index tight.
CREATE INDEX idx_auth_tokens_session_active ON auth_tokens (user_id, idle_expires_at)
    WHERE kind = 'user_session' AND revoked_at IS NULL;
