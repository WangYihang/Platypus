-- refresh_tokens powered the access+refresh JWT pair. Phase-2 auth
-- replaced both halves with opaque pst_ session tokens that store
-- their state in auth_tokens (kind='user_session') and use a
-- sliding-window idle expiry instead of refresh rotation.
--
-- The table is now orphaned: no production writer remains and the
-- session lifecycle has its own DB-backed source of truth. Drop it
-- so a future contributor doesn't accidentally reintroduce a refresh
-- code path on top of stale rows.

DROP INDEX IF EXISTS idx_refresh_tokens_user;
DROP TABLE IF EXISTS refresh_tokens;
