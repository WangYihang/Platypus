-- 000015_rename_enrollment_tokens.up.sql — rename the legacy `pat_tokens`
-- table to reflect what its rows really are: one-shot agent-enrollment
-- credentials.
--
-- The historical name "PAT" (Personal Access Token) was misleading
-- because these tokens cannot be issued by an end user, are not bound
-- to an identity, and burn after a single redemption. A future
-- migration will introduce a true user-PAT surface that needs the
-- "PAT" name; this rename frees it up.
--
-- (The companion `pat_redemption_events` table was already removed in
-- migration 6 when the unified `activities` log absorbed audit duty.)
--
-- Wire surface stays unchanged — the `plt_` token prefix and the
-- `/api/v1/projects/:pid/pat-tokens` admin URL are kept by the Go
-- layer so already-deployed agents and operator scripts keep working.
--
-- The cross-table FK from install_download_tokens.consumed_pat_id is
-- rewritten by SQLite (≥3.26) automatically as part of RENAME TO.
-- A migration test under internal/storage runs PRAGMA foreign_key_check
-- to lock that behaviour in.

ALTER TABLE pat_tokens RENAME TO enrollment_tokens;

DROP INDEX IF EXISTS idx_pat_unrevoked;
CREATE INDEX idx_enrollment_unrevoked
    ON enrollment_tokens(project_id, expires_at) WHERE revoked = 0;
