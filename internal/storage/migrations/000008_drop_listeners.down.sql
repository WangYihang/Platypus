-- 000008_drop_listeners.down.sql — best-effort reverse of the unified
-- ingress migration. The listeners table is recreated and synthesised
-- rows are derived from the distinct ingress_addr values that survived
-- on sessions. The original per-listener metadata (hash_format, public_ip,
-- shell_path, etc.) is gone and cannot be recovered — operators who
-- downgrade past this point should restore from a pre-migration backup
-- instead of relying on this file.
--
-- SQLite's ALTER TABLE ADD COLUMN … REFERENCES is rejected for FK
-- targets, so sessions are rebuilt the same way the up migration
-- rebuilt them, just in reverse.

CREATE TABLE listeners (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    host           TEXT NOT NULL,
    port           INTEGER NOT NULL,
    hash_format    TEXT NOT NULL DEFAULT '',
    disable_history BOOLEAN NOT NULL DEFAULT 0,
    public_ip      TEXT NOT NULL DEFAULT '',
    shell_path     TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Synthesise a listener row per (project_id, ingress_addr) pair. The
-- listener id is deterministic so rerunning the down migration is
-- idempotent.
INSERT INTO listeners (id, project_id, host, port, created_at)
SELECT DISTINCT
    'synth-' || s.project_id || '-' || s.ingress_addr AS id,
    s.project_id,
    CASE
      WHEN instr(s.ingress_addr, ':') > 0
        THEN substr(s.ingress_addr, 1, instr(s.ingress_addr, ':') - 1)
      ELSE s.ingress_addr
    END,
    CASE
      WHEN instr(s.ingress_addr, ':') > 0
        THEN CAST(substr(s.ingress_addr, instr(s.ingress_addr, ':') + 1) AS INTEGER)
      ELSE 0
    END,
    CURRENT_TIMESTAMP
FROM sessions s
WHERE s.ingress_addr <> '';

CREATE TABLE sessions_old (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    listener_id     TEXT NOT NULL REFERENCES listeners(id),
    host_id         TEXT NOT NULL REFERENCES hosts(id),
    alias           TEXT,
    user            TEXT,
    remote_addr     TEXT,
    version         TEXT,
    python2         TEXT,
    python3         TEXT,
    interfaces_json TEXT,
    group_dispatch  BOOLEAN NOT NULL DEFAULT 0,
    connected_at    DATETIME NOT NULL,
    disconnected_at DATETIME
);

INSERT INTO sessions_old (
    id, project_id, listener_id, host_id,
    alias, user, remote_addr, version, python2, python3, interfaces_json,
    group_dispatch, connected_at, disconnected_at
)
SELECT
    s.id, s.project_id,
    COALESCE(
      (SELECT l.id FROM listeners l
        WHERE l.project_id = s.project_id
          AND (l.host || ':' || l.port) = s.ingress_addr
        LIMIT 1),
      ''
    ),
    s.host_id,
    s.alias, s.user, s.remote_addr, s.version, s.python2, s.python3, s.interfaces_json,
    s.group_dispatch, s.connected_at, s.disconnected_at
FROM sessions s;

DROP INDEX IF EXISTS idx_sessions_project_connected;
DROP INDEX IF EXISTS idx_sessions_live;
DROP INDEX IF EXISTS idx_sessions_host;
DROP TABLE sessions;

ALTER TABLE sessions_old RENAME TO sessions;

CREATE INDEX idx_sessions_host ON sessions(host_id);
CREATE INDEX idx_sessions_listener ON sessions(listener_id);
CREATE INDEX idx_sessions_live ON sessions(project_id) WHERE disconnected_at IS NULL;
CREATE INDEX idx_sessions_project_connected
  ON sessions(project_id, connected_at DESC);
