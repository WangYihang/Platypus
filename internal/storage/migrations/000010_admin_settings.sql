-- 000010_admin_settings.up.sql — generic key/value store for
-- runtime-tunable admin settings (token TTLs, release channel,
-- presigned URL TTL, mesh discovery flags). Values are JSON-encoded
-- strings; the registry layer in internal/settings parses them into
-- typed Go values. A row exists only when the admin has explicitly
-- overridden a YAML default via the Web UI; absence falls back to
-- the YAML value, then to the hardcoded default.

CREATE TABLE admin_settings (
    key         TEXT     PRIMARY KEY,
    value       TEXT     NOT NULL,
    updated_at  DATETIME NOT NULL,
    updated_by  TEXT     NOT NULL REFERENCES users(id)
);
