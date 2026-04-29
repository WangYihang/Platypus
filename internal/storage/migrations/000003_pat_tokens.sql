-- 000003_pat_tokens.up.sql — agent enrollment credentials (PAT) + session
-- tokens, plus the append-only audit tables that cover every state change.
--
-- Design principles:
--   1. No row ever gets physically deleted. "Status" of a PAT or a session
--      is derived from (revoked, expires_at, uses, rotated_at) — we never
--      materialise a status column that can drift from the underlying facts.
--   2. Every non-trivial action (issue, redeem, rotate, revoke) writes an
--      append-only event row. Mutable state tables are for fast queries;
--      event tables are the audit truth.
--   3. Event tables have no FKs back to the state tables. That's deliberate:
--      we want to record failed redemptions against token_ids that don't
--      exist (scan / brute-force attempts) without the FK rejecting them.

-- Personal/Provisioning Access Tokens. One row per issued PAT, ever.
-- Status is derived — callers filter by (revoked, expires_at, uses, max_uses).
CREATE TABLE pat_tokens (
    token_id           TEXT PRIMARY KEY,
    secret_hash        BLOB NOT NULL,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    issued_by_user     TEXT NOT NULL REFERENCES users(id),
    issued_at          DATETIME NOT NULL,
    expires_at         DATETIME NOT NULL,
    max_uses           INTEGER NOT NULL DEFAULT 1,
    uses               INTEGER NOT NULL DEFAULT 0,
    binding_machine_id TEXT,
    binding_host_alias TEXT,
    description        TEXT,
    revoked            INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
    revoked_at         DATETIME,
    revoked_by_user    TEXT REFERENCES users(id),
    revoked_reason     TEXT,
    CHECK (max_uses >= 1),
    CHECK (uses >= 0 AND uses <= max_uses),
    CHECK (revoked = 0 OR revoked_at IS NOT NULL)
);

-- Partial index on unrevoked rows (revoked=0 is a deterministic predicate;
-- including expires_at in the key lets the common "find live tokens"
-- query seek both conditions).
CREATE INDEX idx_pat_unrevoked
    ON pat_tokens(project_id, expires_at)
    WHERE revoked = 0;

-- Agent sessions. Append-only: each rotation inserts a new row and marks
-- the previous one as rotated_at. revoked_at marks administrator kill.
-- A session is "active" iff rotated_at IS NULL AND revoked_at IS NULL AND
-- expires_at > now(). The partial UNIQUE index enforces that invariant at
-- the DB level — any attempt to have two active sessions for the same
-- agent fails with a UNIQUE violation.
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

-- Every PAT redemption attempt — success or failure. outcome enumerates
-- the possible outcomes, matched 1:1 to the reasons RedeemCredential can
-- reject on. No FK to pat_tokens: we must record attempts against
-- non-existent token_ids (scanning attacks) verbatim.
CREATE TABLE pat_redemption_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    at           DATETIME NOT NULL,
    token_id     TEXT NOT NULL,
    client_ip    TEXT,
    machine_id   TEXT,
    hostname     TEXT,
    agent_id     TEXT,
    outcome      TEXT NOT NULL,
    error_detail TEXT,
    CHECK (outcome IN (
        'success',
        'invalid_secret',
        'unknown_token',
        'expired',
        'revoked',
        'max_uses_reached',
        'binding_machine_mismatch',
        'malformed'
    ))
);
CREATE INDEX idx_pat_redemption_token ON pat_redemption_events(token_id, at);
CREATE INDEX idx_pat_redemption_ip    ON pat_redemption_events(client_ip, at);
CREATE INDEX idx_pat_redemption_time  ON pat_redemption_events(at);

-- Agent-side connection events. These complement agent_sessions: the
-- session table keeps mutable last_seen_at for convenience, but the full
-- reconnect timeline lives here.
CREATE TABLE agent_connection_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    at           DATETIME NOT NULL,
    agent_id     TEXT,
    session_id   TEXT,
    client_ip    TEXT,
    event_type   TEXT NOT NULL,
    reason       TEXT,
    transport    TEXT,
    CHECK (event_type IN (
        'enroll_success',
        'enroll_reject',
        'reconnect_success',
        'reconnect_reject',
        'disconnect'
    )),
    CHECK (transport IS NULL OR transport IN ('tls_direct', 'mesh'))
);
CREATE INDEX idx_conn_events_agent ON agent_connection_events(agent_id, at);
CREATE INDEX idx_conn_events_time  ON agent_connection_events(at);

-- Administrator actions on PAT / session / agent resources. This is the
-- primary audit trail for operator accountability.
CREATE TABLE admin_audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    at           DATETIME NOT NULL,
    actor_user   TEXT NOT NULL REFERENCES users(id),
    actor_ip     TEXT,
    actor_ua     TEXT,
    action       TEXT NOT NULL,
    target_type  TEXT,
    target_id    TEXT,
    project_id   TEXT,
    details      TEXT,
    outcome      TEXT NOT NULL,
    error        TEXT,
    CHECK (outcome IN ('success', 'denied', 'error'))
);
CREATE INDEX idx_admin_audit_actor  ON admin_audit_log(actor_user, at);
CREATE INDEX idx_admin_audit_target ON admin_audit_log(target_type, target_id, at);
CREATE INDEX idx_admin_audit_time   ON admin_audit_log(at);

-- PII redaction log. If regulation requires clearing a PII column in one
-- of the event tables, the operator sets that column to "[redacted-...]"
-- and records the action here. Event rows themselves stay present, so
-- counts and time series remain truthful.
CREATE TABLE redaction_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    at          DATETIME NOT NULL,
    actor_user  TEXT NOT NULL REFERENCES users(id),
    table_name  TEXT NOT NULL,
    row_id      TEXT NOT NULL,
    column_name TEXT NOT NULL,
    reason      TEXT NOT NULL
);
CREATE INDEX idx_redaction_time ON redaction_log(at);
