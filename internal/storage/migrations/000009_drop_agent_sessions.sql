-- 000009_drop_agent_sessions.up.sql — retire the server-side session-
-- token store. After the v2 refactor, agent identity is carried
-- exclusively by the project-CA-signed client certificate issued at
-- enrollment (see internal/pki + handler_enroll_v2.go). No code path
-- reads or writes agent_sessions any more; the store code in
-- internal/storage/agent_sessions.go was deleted in the previous
-- commit.
--
-- Dropping the table reclaims disk + removes the orphan schema. FK
-- edges from agent_sessions referenced projects(id) and users(id)
-- (as revoked_by_user); those target tables are untouched.

DROP INDEX IF EXISTS idx_agent_session_one_active;
DROP INDEX IF EXISTS idx_agent_session_history;
DROP INDEX IF EXISTS idx_agent_session_project;

DROP TABLE IF EXISTS agent_sessions;
