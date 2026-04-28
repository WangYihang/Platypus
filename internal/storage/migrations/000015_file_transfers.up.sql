-- 000015_file_transfers.up.sql — persistent record of file-transfer
-- tasks (downloads + uploads) so the UI can show in-flight progress
-- and historical archives in one consolidated list.
--
-- Every row is created when a user kicks off a transfer (download or
-- upload) and updated as bytes flow. Status transitions:
--   pending → running → done | failed | canceled
--
-- We keep rows forever (per product decision) so users can audit
-- past transfers indefinitely; pruning happens via dedicated retention
-- queries elsewhere if ever needed.
--
-- project_id and host_id are NOT NULL because every transfer is
-- always scoped to a specific project + agent. user_id is also
-- NOT NULL because every transfer is initiated by an authenticated
-- user (including system-issued admin operations).
--
-- paths is a JSON array of the source filesystem paths the transfer
-- references; format is the encoding ("tar.gz" / "tar" / "zip" /
-- "raw" for single-file downloads). bytes_transferred is updated
-- continuously while running; total_bytes is set after the
-- pre-scan and may be NULL when scanning was skipped.
--
-- Indexes cover the two main queries the UI runs: the project-wide
-- transfers tab (project_id, started_at DESC) and the per-host tab
-- (host_id, started_at DESC).

CREATE TABLE file_transfers (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    host_id           TEXT NOT NULL,
    user_id           TEXT NOT NULL REFERENCES users(id),
    direction         TEXT NOT NULL,
    kind              TEXT NOT NULL,
    format            TEXT NOT NULL DEFAULT '',
    paths_json        TEXT NOT NULL,
    status            TEXT NOT NULL,
    bytes_transferred INTEGER NOT NULL DEFAULT 0,
    total_bytes       INTEGER,
    error_message     TEXT NOT NULL DEFAULT '',
    started_at        DATETIME NOT NULL,
    finished_at       DATETIME,
    CHECK (direction IN ('download', 'upload')),
    CHECK (kind IN ('file', 'archive', 'folder')),
    CHECK (status IN ('pending', 'running', 'done', 'failed', 'canceled'))
);

CREATE INDEX idx_file_transfers_project_started
    ON file_transfers(project_id, started_at DESC);

CREATE INDEX idx_file_transfers_host_started
    ON file_transfers(host_id, started_at DESC);

CREATE INDEX idx_file_transfers_status
    ON file_transfers(status);
