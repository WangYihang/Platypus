-- 000033_enrollment_presets.up.sql — reusable enrollment configurations.
--
-- A preset captures the choices an operator makes in the Enroll Agent
-- wizard (target OS/arch, TTL, PAT max-uses, baseline plugins, …) so
-- repeat enrolments collapse to "pick preset → click Generate" instead
-- of walking the 11-step flow. Presets do NOT carry the issued PAT
-- itself — every redemption still mints a fresh single-use install
-- token via the existing install-artifact endpoint. This row is purely
-- a saved input template.
--
-- `is_seed` flags the three system defaults (Linux x86_64 / Windows x64
-- / macOS Apple Silicon) that get lazily inserted on first wizard open
-- of a fresh project. Operators can edit or delete them; once deleted,
-- the project's preset list is non-empty so the seed step won't fire
-- again — by design, "I deleted that one on purpose" sticks.

CREATE TABLE enrollment_presets (
    preset_id              TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                   TEXT NOT NULL,
    description            TEXT,
    server_endpoint        TEXT,
    target_os              TEXT,
    target_arch            TEXT,
    ttl_seconds            INTEGER,
    pat_max_uses           INTEGER,
    auto_approve           INTEGER NOT NULL DEFAULT 0 CHECK (auto_approve IN (0,1)),
    skip_tls_verification  INTEGER NOT NULL DEFAULT 1 CHECK (skip_tls_verification IN (0,1)),
    -- baseline_plugin_ids is a JSON array of plugin id strings; NULL is
    -- treated as the empty list. JSON-in-TEXT is the existing
    -- convention in this schema for small unbounded string sets.
    baseline_plugin_ids    TEXT,
    pat_description        TEXT,
    is_seed                INTEGER NOT NULL DEFAULT 0 CHECK (is_seed IN (0,1)),
    created_by_user        TEXT REFERENCES users(id),
    created_at             DATETIME NOT NULL,
    updated_at             DATETIME NOT NULL,
    CHECK (ttl_seconds IS NULL OR ttl_seconds > 0),
    CHECK (pat_max_uses IS NULL OR pat_max_uses >= 1)
);

-- Names are unique within a project so seed-on-empty stays idempotent
-- via INSERT OR IGNORE keyed on (project_id, name).
CREATE UNIQUE INDEX idx_enrollment_presets_name
    ON enrollment_presets(project_id, name);

-- Listing query is "all presets in a project, newest first".
CREATE INDEX idx_enrollment_presets_list
    ON enrollment_presets(project_id, created_at DESC);
