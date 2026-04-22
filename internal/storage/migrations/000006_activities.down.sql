DROP INDEX IF EXISTS idx_activities_target;
DROP INDEX IF EXISTS idx_activities_actor_at;
DROP INDEX IF EXISTS idx_activities_session;
DROP INDEX IF EXISTS idx_activities_project_actor;
DROP INDEX IF EXISTS idx_activities_project_action;
DROP INDEX IF EXISTS idx_activities_project_cat_at;
DROP INDEX IF EXISTS idx_activities_project_at;
DROP INDEX IF EXISTS idx_activities_at;
DROP TABLE IF EXISTS activities;

-- Re-create the three tables that 000003 originally introduced, so that
-- rolling 000006 down returns the schema to the 000005 state. Bodies
-- mirror 000003_pat_tokens.up.sql verbatim.
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
