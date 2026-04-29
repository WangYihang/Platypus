-- Reverse 000022_host_approval. SQLite doesn't support DROP COLUMN
-- on older versions, but the project's modernc.org/sqlite is recent
-- enough (3.45+) that ALTER TABLE DROP COLUMN works directly.

DROP INDEX IF EXISTS idx_hosts_approval_pending;

ALTER TABLE hosts DROP COLUMN approval_reason;
ALTER TABLE hosts DROP COLUMN approval_decided_by;
ALTER TABLE hosts DROP COLUMN approval_decided_at;
ALTER TABLE hosts DROP COLUMN approval_status;

ALTER TABLE enrollment_tokens DROP COLUMN auto_approve;
ALTER TABLE install_download_tokens DROP COLUMN auto_approve;
