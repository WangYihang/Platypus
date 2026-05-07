-- 000034_plugin_specs.up.sql — unified PluginSpec atom for plugin
-- deployment intent.
--
-- Before this migration, three carriers held three orthogonal pieces:
--   · enrollment_presets.baseline_plugin_ids       JSON []string
--   · install_download_tokens.baseline_plugin_ids  JSON []string
--   · hosts.baseline_plugin_ids                    JSON []string
-- "what each plugin is allowed to do" lived in the wire request only,
-- and "how the plugin is configured" had no carrier at all.
--
-- This migration consolidates everything into PluginSpec — one atom
-- carrying plugin_id + version + granted_capabilities + config_overrides
-- + schema_version. The atom serialises identically wherever a plugin
-- deployment intent appears, removing the per-layer translation glue.
--
-- Two additional tables come along:
--   · project_secrets — encrypted at rest under the project KEK; fields
--     in plugin configs reference these by id.
--   · host_plugins    — first-class queryable state for "what's
--     running where with what config".
--
-- Existing baseline_plugin_ids values are migrated row-by-row into the
-- minimal PluginSpec shape: ["sys-info", "shell"] becomes
-- [{"plugin_id":"sys-info"}, {"plugin_id":"shell"}]. Granted
-- capabilities, config, and schema_version stay unset (NULL / 0)
-- because the old shape didn't carry them; operators re-author through
-- the new editor when they want to customise.

-- 1. enrollment_presets: add plugin_specs, migrate, drop old.
ALTER TABLE enrollment_presets ADD COLUMN plugin_specs TEXT;

UPDATE enrollment_presets
   SET plugin_specs = (
       SELECT json_group_array(json_object('plugin_id', value))
         FROM json_each(enrollment_presets.baseline_plugin_ids)
   )
 WHERE baseline_plugin_ids IS NOT NULL
   AND baseline_plugin_ids != ''
   AND baseline_plugin_ids != '[]';

ALTER TABLE enrollment_presets DROP COLUMN baseline_plugin_ids;

-- 2. install_download_tokens: this column stored CSV-encoded plugin
-- ids (TEXT NOT NULL DEFAULT '' from migration 000031), not JSON, so
-- we compose a JSON array via printf/replace before json_each-ing it.
-- Plugin ids are constrained to [.\-_a-z0-9] (reverse-DNS style), so
-- splitting on comma is unambiguous.
ALTER TABLE install_download_tokens ADD COLUMN plugin_specs TEXT;

UPDATE install_download_tokens
   SET plugin_specs = (
       SELECT json_group_array(json_object('plugin_id', value))
         FROM json_each(
             '["' || replace(install_download_tokens.baseline_plugin_ids, ',', '","') || '"]'
         )
   )
 WHERE baseline_plugin_ids IS NOT NULL
   AND baseline_plugin_ids != '';

ALTER TABLE install_download_tokens DROP COLUMN baseline_plugin_ids;

-- 3. hosts: same CSV-to-JSON conversion. The hosts column captures
-- what a host enrolled with; converting to the rich shape lets the
-- eventual host_plugins reconciler consume the same atom we send
-- everywhere else.
ALTER TABLE hosts ADD COLUMN plugin_specs TEXT;

UPDATE hosts
   SET plugin_specs = (
       SELECT json_group_array(json_object('plugin_id', value))
         FROM json_each(
             '["' || replace(hosts.baseline_plugin_ids, ',', '","') || '"]'
         )
   )
 WHERE baseline_plugin_ids IS NOT NULL
   AND baseline_plugin_ids != '';

ALTER TABLE hosts DROP COLUMN baseline_plugin_ids;

-- 4. project_secrets — small append-mostly store for sensitive plugin
-- config values. Encryption format mirrors project_ca: AES-256-GCM
-- under the operator-supplied PLATYPUS_CA_KEK; storage carries the
-- 12-byte nonce alongside the ciphertext so decryption is
-- self-contained.
--
-- Values are immutable once written. Rotations create a new row with a
-- new secret_id; the old row is marked revoked rather than overwritten
-- so audit trails stay readable even after the underlying secret is
-- replaced. last_used_at is updated opportunistically by the resolver
-- on read.
CREATE TABLE project_secrets (
    secret_id        TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    description      TEXT,
    nonce            BLOB NOT NULL,
    ciphertext       BLOB NOT NULL,
    created_by_user  TEXT REFERENCES users(id),
    created_at       DATETIME NOT NULL,
    last_used_at     DATETIME,
    revoked          INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
    revoked_at       DATETIME,
    revoked_by_user  TEXT REFERENCES users(id),
    revoked_reason   TEXT,
    CHECK (length(nonce) = 12),
    CHECK (revoked = 0 OR revoked_at IS NOT NULL)
);

-- (project_id, name) UNIQUE WHERE revoked=0 — operators reuse names
-- after rotation. We can't put this on the table because partial
-- UNIQUE indexes require the index syntax.
CREATE UNIQUE INDEX idx_project_secrets_active_name
    ON project_secrets(project_id, name)
    WHERE revoked = 0;

CREATE INDEX idx_project_secrets_list
    ON project_secrets(project_id, created_at DESC);

-- 5. host_plugins — first-class state of "what plugins are installed
-- on which host with which config". Composite primary key (host_id,
-- plugin_id) gives us per-host upsert semantics: re-installing the
-- same plugin updates the row in place rather than appending.
--
-- spec_json holds the resolved (post-secret-substitution) PluginSpec —
-- the same shape we shipped to the agent. Storing the resolved form
-- means an audit trail of "what config was actually deployed" stays
-- readable even after the underlying secret rotates. Resolved configs
-- never contain {"$secret":"..."} refs; values are substituted in.
--
-- state encodes the install lifecycle:
--   pending   — the server queued an install, the agent hasn't ack'd
--   installed — the agent confirms running this version with this config
--   failed    — the install attempt errored; last_error has the detail
--   removed   — the operator uninstalled; row kept for audit history
CREATE TABLE host_plugins (
    host_id          TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    plugin_id        TEXT NOT NULL,
    version          TEXT NOT NULL,
    granted_capabilities TEXT,    -- JSON []string
    config_resolved  TEXT,        -- JSON object — post-secret-substitution
    schema_version   INTEGER NOT NULL DEFAULT 0,
    state            TEXT NOT NULL CHECK (state IN ('pending', 'installed', 'failed', 'removed')),
    installed_at     DATETIME,
    updated_at       DATETIME NOT NULL,
    last_error       TEXT,
    PRIMARY KEY (host_id, plugin_id)
);

CREATE INDEX idx_host_plugins_by_plugin
    ON host_plugins(plugin_id, version);

CREATE INDEX idx_host_plugins_by_host_state
    ON host_plugins(host_id, state);
