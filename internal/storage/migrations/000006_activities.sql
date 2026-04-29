-- 000006_activities.up.sql — replace three narrow audit tables with one
-- unified global activity log.
--
-- Design goals:
--   1. Every meaningful action in the system (user, admin, agent, server)
--      lands as exactly one append-only row here. One timeline.
--   2. Common, queryable dimensions are first-class columns (at, project_id,
--      actor_*, category, action, target_*, outcome, duration_ms, session_id,
--      request_id). Whatever varies per action shape lives in the `meta`
--      JSON blob — SQLite's json1 extension handles ad-hoc access when
--      needed.
--   3. project_id is NULLABLE because many events (user.login, server.start,
--      project.create) happen outside any project scope.
--   4. No FKs to state tables: we must record events against
--      users/projects/tokens that no longer exist (or never did — scanning
--      attacks). The rows are history, not derived state.
--
-- The three previous audit tables (pat_redemption_events,
-- agent_connection_events, admin_audit_log) are dropped here; their prior
-- responsibilities fold into rows with distinct (category, action) values.

DROP INDEX IF EXISTS idx_admin_audit_time;
DROP INDEX IF EXISTS idx_admin_audit_target;
DROP INDEX IF EXISTS idx_admin_audit_actor;
DROP TABLE IF EXISTS admin_audit_log;

DROP INDEX IF EXISTS idx_conn_events_time;
DROP INDEX IF EXISTS idx_conn_events_agent;
DROP TABLE IF EXISTS agent_connection_events;

DROP INDEX IF EXISTS idx_pat_redemption_time;
DROP INDEX IF EXISTS idx_pat_redemption_ip;
DROP INDEX IF EXISTS idx_pat_redemption_token;
DROP TABLE IF EXISTS pat_redemption_events;

CREATE TABLE activities (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    at             DATETIME NOT NULL,
    project_id     TEXT,
    actor_type     TEXT NOT NULL,
    actor_user     TEXT,
    actor_ip       TEXT,
    actor_ua       TEXT,
    actor_token_id TEXT,
    category       TEXT NOT NULL,
    action         TEXT NOT NULL,
    target_type    TEXT,
    target_id      TEXT,
    target_label   TEXT,
    outcome        TEXT NOT NULL,
    error          TEXT,
    duration_ms    INTEGER,
    request_id     TEXT,
    session_id     TEXT,
    meta           TEXT,
    created_at     DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK (outcome IN ('success', 'denied', 'error')),
    CHECK (actor_type IN ('user', 'system', 'agent', 'api_token', 'anonymous'))
);

CREATE INDEX idx_activities_at               ON activities(at DESC);
CREATE INDEX idx_activities_project_at       ON activities(project_id, at DESC);
CREATE INDEX idx_activities_project_cat_at   ON activities(project_id, category, at DESC);
CREATE INDEX idx_activities_project_action   ON activities(project_id, action, at DESC);
CREATE INDEX idx_activities_project_actor    ON activities(project_id, actor_user, at DESC);
CREATE INDEX idx_activities_session          ON activities(session_id);
CREATE INDEX idx_activities_actor_at         ON activities(actor_user, at DESC);
CREATE INDEX idx_activities_target           ON activities(target_type, target_id, at DESC);
