-- Drop in reverse order — the append-only event tables first, then the
-- mutable state tables they reference by name.
DROP INDEX IF EXISTS idx_redaction_time;
DROP TABLE IF EXISTS redaction_log;

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

DROP INDEX IF EXISTS idx_agent_session_project;
DROP INDEX IF EXISTS idx_agent_session_history;
DROP INDEX IF EXISTS idx_agent_session_one_active;
DROP TABLE IF EXISTS agent_sessions;

DROP INDEX IF EXISTS idx_pat_unrevoked;
DROP TABLE IF EXISTS pat_tokens;
