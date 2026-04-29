-- 000017_drop_aat.up.sql — retire the AAT (AI Agent Token) surface.
--
-- AAT was an admin-only experimental feature; the new user-self PAT
-- (introduced in migration 18) covers the same use case in a more
-- principled way (issued by the holder, no admin gating, no project
-- binding). No production deployment relied on AAT, so we delete the
-- rows rather than migrate them.
--
-- The CHECK constraint on auth_tokens.kind still permits 'aat' until
-- migration 18 rebuilds the table — SQLite cannot ALTER a CHECK in
-- place, and rebuilding twice (once here, once in 17) is wasteful.
-- The kind-active index is dropped here because the kind itself is
-- now dead and the index would only collect dust.

DELETE FROM auth_tokens WHERE kind = 'aat';

DROP INDEX IF EXISTS idx_auth_tokens_aat_active;
