-- auth_tokens is the unified store for every opaque credential the
-- server hands out. Today: kind='aat' (AI agent tokens, long-lived,
-- scoped) and kind='user_session' (browser sessions replacing JWT,
-- sliding-window). Both share 95% of the columns — secret_hash,
-- expiries, last-used metadata, revocation — and two CHECK clauses
-- enforce the kind-specific shape so SQL itself rejects mismatched
-- writes. PAT and install tokens stay in their own tables: they have
-- one-shot consumption and download-tracking semantics that don't
-- belong here.

CREATE TABLE auth_tokens (
    -- Identity. Includes the wire-format prefix ("aat_..." / "pst_...")
    -- so id is self-describing in logs and stays unique across kinds
    -- without a compound key.
    token_id          TEXT PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('aat', 'user_session')),
    secret_hash       BLOB NOT NULL,

    -- Ownership: AAT.user_id is the issuer / creator; user_session.user_id
    -- is the session subject. ON DELETE CASCADE ensures deleting a user
    -- nukes their AATs and sessions atomically — no orphaned principals.
    user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Display / audit metadata. AAT must have a name (per CHECK below);
    -- sessions don't.
    name              TEXT,
    description       TEXT,

    -- Lifecycle. expires_at is the hard upper bound for both kinds.
    -- last_used_* are best-effort touched after each successful Verify.
    created_at        DATETIME NOT NULL,
    expires_at        DATETIME NOT NULL,
    last_used_at      DATETIME,
    last_used_ip      TEXT,
    user_agent        TEXT,

    -- Revocation. revoked_at != NULL is the single source of truth for
    -- "is this token alive". Reason / actor are optional but recorded
    -- on every admin-initiated revoke so the audit log can join.
    revoked_at        DATETIME,
    revoked_by_user   TEXT REFERENCES users(id),
    revoked_reason    TEXT,

    -- AAT-specific. project_id NULL = global token. role caps the
    -- maximum authority the issuer could grant; scopes is the actual
    -- granted set (space-delimited).
    project_id        TEXT REFERENCES projects(id) ON DELETE CASCADE,
    role              TEXT CHECK (role IS NULL OR role IN ('admin', 'operator', 'viewer')),
    scopes            TEXT,

    -- user_session-specific sliding window: every successful Verify
    -- pushes this forward; any request after idle_expires_at fails
    -- even if expires_at hasn't fired yet.
    idle_expires_at   DATETIME,

    -- Shape integrity. Each kind enforces its own field presence so
    -- the table can't drift into "AAT row with no scopes" or "session
    -- with a role attached".
    CHECK (
        (kind = 'aat'
            AND name IS NOT NULL
            AND role IS NOT NULL
            AND scopes IS NOT NULL
            AND idle_expires_at IS NULL)
     OR
        (kind = 'user_session'
            AND project_id IS NULL
            AND role IS NULL
            AND scopes IS NULL
            AND idle_expires_at IS NOT NULL)
    )
);

-- Listing tokens for a user (settings UI / admin audit). DESC on
-- created_at so the newest entries come first without an in-app sort.
CREATE INDEX idx_auth_tokens_user_kind ON auth_tokens (user_id, kind, created_at DESC);

-- Project-scoped active AAT enumeration. Partial so the index covers
-- only live AAT rows; user-session rows never touch this index.
CREATE INDEX idx_auth_tokens_aat_active ON auth_tokens (project_id, expires_at)
    WHERE kind = 'aat' AND revoked_at IS NULL;

-- Active session enumeration / GC for a user. Same partial-index
-- trick keeps the index tight.
CREATE INDEX idx_auth_tokens_session_active ON auth_tokens (user_id, idle_expires_at)
    WHERE kind = 'user_session' AND revoked_at IS NULL;
