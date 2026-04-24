-- 000009_drop_agent_sessions.down.sql — recreate the agent_sessions
-- table + its three indexes exactly as 000003_pat_tokens.up.sql
-- defined them, in case an operator needs to roll the server back.
-- Data is NOT restored; the rows are gone once the up migration
-- runs. Operators who downgrade through this point should restore
-- from a pre-migration backup if they need the history.

CREATE TABLE agent_sessions (
    session_id          TEXT PRIMARY KEY,
    agent_id            TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_token_hash  BLOB NOT NULL,
    issued_at           DATETIME NOT NULL,
    issued_reason       TEXT NOT NULL CHECK (issued_reason IN ('enroll', 'rotation', 'reset')),
    rotated_from        TEXT REFERENCES agent_sessions(session_id),
    expires_at          DATETIME NOT NULL,
    rotated_at          DATETIME,
    revoked_at          DATETIME,
    revoked_reason      TEXT,
    revoked_by_user     TEXT REFERENCES users(id),
    last_seen_at        DATETIME,
    last_seen_ip        TEXT,
    machine_id          TEXT,
    CHECK ((rotated_at IS NULL) OR (revoked_at IS NULL))
);

CREATE UNIQUE INDEX idx_agent_session_one_active
    ON agent_sessions(agent_id)
    WHERE rotated_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_agent_session_history
    ON agent_sessions(agent_id, issued_at DESC);
CREATE INDEX idx_agent_session_project
    ON agent_sessions(project_id, issued_at DESC);
