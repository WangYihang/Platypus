-- Reverse of 000016: recreate the project-scoped active-AAT partial
-- index that migration 17 dropped. Row-level data isn't restored —
-- the up migration's DELETE is destructive by design (no production
-- AAT users existed) so a downgrade only puts the index back so the
-- pre-16 schema lines up byte-for-byte.

CREATE INDEX IF NOT EXISTS idx_auth_tokens_aat_active ON auth_tokens (project_id, expires_at)
    WHERE kind = 'aat' AND revoked_at IS NULL;
