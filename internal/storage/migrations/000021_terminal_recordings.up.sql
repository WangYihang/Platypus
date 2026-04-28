-- 000021_terminal_recordings.up.sql — persistent record of every
-- interactive shell session opened by an operator against an agent.
--
-- One row per shell. The actual playback bytes live on disk under
-- <recordings_dir>/<id>.cast in asciinema v2 format; the row stores
-- pointer + summary metadata so the UI list page can render without
-- touching the filesystem (cards show user, host, duration, size,
-- rough activity bursts) and the preview endpoint can stream the
-- file directly.
--
-- Status transitions:
--   recording → completed
--   recording → failed
-- Only `recording` rows have a NULL ended_at; rows with a non-NULL
-- ended_at are immutable from the recorder's side (delete is the
-- only mutation).
--
-- All references are nominally scoped: project_id + host_id + user_id
-- gate row visibility through the same RBAC chain the rest of the
-- per-project surface uses. file_path is stored relative to the
-- configured recordings dir so the data volume can be remounted under
-- a different absolute path without rewriting rows.
--
-- title is a human-editable label (defaults to ""): "rotating wp creds
-- on web-04", "investigating cron failure", etc. Frontend will let
-- operators set it after-the-fact for searchability.
--
-- Indexes cover the project recordings list (project_id, started_at
-- DESC) and the per-host filter (host_id, started_at DESC). The
-- per-user filter rides idx_recordings_project_started since the
-- list page already filters by project first.

CREATE TABLE terminal_recordings (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    host_id       TEXT NOT NULL,
    agent_id      TEXT NOT NULL DEFAULT '',
    user_id       TEXT NOT NULL DEFAULT '',
    cols          INTEGER NOT NULL DEFAULT 80,
    rows          INTEGER NOT NULL DEFAULT 24,
    shell         TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    file_path     TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    frame_count   INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'recording',
    error_message TEXT NOT NULL DEFAULT '',
    started_at    DATETIME NOT NULL,
    ended_at      DATETIME,
    CHECK (status IN ('recording', 'completed', 'failed'))
);

CREATE INDEX idx_recordings_project_started
    ON terminal_recordings(project_id, started_at DESC);

CREATE INDEX idx_recordings_host_started
    ON terminal_recordings(host_id, started_at DESC);

CREATE INDEX idx_recordings_user_started
    ON terminal_recordings(user_id, started_at DESC);
