-- Reverse of 000017: rebuild auth_tokens with the migration-13 CHECK
-- (kind IN ('aat', 'user_session')) so a downgrade restores the
-- pre-PAT shape. Any kind='pat' rows are dropped — they couldn't
-- exist under the old CHECK anyway.

CREATE TABLE auth_tokens_old (
    token_id          TEXT PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('aat', 'user_session')),
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
    role              TEXT CHECK (role IS NULL OR role IN ('admin', 'operator', 'viewer')),
    scopes            TEXT,
    idle_expires_at   DATETIME,
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

INSERT INTO auth_tokens_old SELECT
    token_id, kind, secret_hash, user_id,
    name, description,
    created_at, expires_at,
    last_used_at, last_used_ip, user_agent,
    revoked_at, revoked_by_user, revoked_reason,
    project_id, role, scopes,
    idle_expires_at
FROM auth_tokens
WHERE kind <> 'pat';

DROP TABLE auth_tokens;
ALTER TABLE auth_tokens_old RENAME TO auth_tokens;

CREATE INDEX idx_auth_tokens_user_kind ON auth_tokens (user_id, kind, created_at DESC);
CREATE INDEX idx_auth_tokens_session_active ON auth_tokens (user_id, idle_expires_at)
    WHERE kind = 'user_session' AND revoked_at IS NULL;
