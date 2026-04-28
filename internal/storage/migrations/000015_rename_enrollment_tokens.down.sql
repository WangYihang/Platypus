-- Reverse of 000015: rename enrollment_tokens back to pat_tokens so a
-- downward migration restores the exact schema migration 14 left behind.

ALTER TABLE enrollment_tokens RENAME TO pat_tokens;

DROP INDEX IF EXISTS idx_enrollment_unrevoked;
CREATE INDEX idx_pat_unrevoked
    ON pat_tokens(project_id, expires_at) WHERE revoked = 0;
