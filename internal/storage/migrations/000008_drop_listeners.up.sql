-- 000008_drop_listeners.up.sql — retire the listeners table and the
-- per-session FK that pinned every session to one. The unified ingress
-- dispatcher (internal/ingress) routes every agent through a single TLS
-- port; storage.Listener rows no longer carry operational meaning.
--
-- Sessions keep a free-form ingress_addr string ("host:port") so the
-- audit trail survives the table drop.
--
-- SQLite can't ALTER TABLE DROP COLUMN on a column with an FK
-- reference, so the sessions rebuild is the classic create-new /
-- insert-select / drop-old / rename dance.

CREATE TABLE sessions_new (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id),
    host_id          TEXT NOT NULL REFERENCES hosts(id),
    ingress_addr     TEXT NOT NULL DEFAULT '',
    alias            TEXT,
    user             TEXT,
    remote_addr      TEXT,
    version          TEXT,
    python2          TEXT,
    python3          TEXT,
    interfaces_json  TEXT,
    group_dispatch   BOOLEAN NOT NULL DEFAULT 0,
    connected_at     DATETIME NOT NULL,
    disconnected_at  DATETIME
);

INSERT INTO sessions_new (
    id, project_id, host_id, ingress_addr,
    alias, user, remote_addr, version, python2, python3, interfaces_json,
    group_dispatch, connected_at, disconnected_at
)
SELECT
    s.id, s.project_id, s.host_id,
    COALESCE(
      (SELECT l.host || ':' || l.port FROM listeners l WHERE l.id = s.listener_id),
      ''
    ),
    s.alias, s.user, s.remote_addr, s.version, s.python2, s.python3, s.interfaces_json,
    s.group_dispatch, s.connected_at, s.disconnected_at
FROM sessions s;

DROP INDEX IF EXISTS idx_sessions_project_connected;
DROP INDEX IF EXISTS idx_sessions_live;
DROP INDEX IF EXISTS idx_sessions_listener;
DROP INDEX IF EXISTS idx_sessions_host;
DROP TABLE sessions;

ALTER TABLE sessions_new RENAME TO sessions;

CREATE INDEX idx_sessions_host ON sessions(host_id);
CREATE INDEX idx_sessions_live ON sessions(project_id) WHERE disconnected_at IS NULL;
CREATE INDEX idx_sessions_project_connected
  ON sessions(project_id, connected_at DESC);

DROP TABLE IF EXISTS listeners;
