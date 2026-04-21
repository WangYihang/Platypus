-- 000001_init.up.sql — initial schema for the Projects / Hosts / Sessions
-- hierarchy plus the JWT-backed user/role system. Everything lives in one
-- migration so a fresh install is a single atomic step.

CREATE TABLE users (
    id              TEXT PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at      DATETIME NOT NULL,
    last_login_at   DATETIME
);

CREATE TABLE refresh_tokens (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at      DATETIME NOT NULL,
    revoked_at      DATETIME
);

CREATE TABLE projects (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    created_at      DATETIME NOT NULL,
    created_by      TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE project_members (
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE listeners (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    host            TEXT NOT NULL,
    port            INTEGER NOT NULL,
    public_ip       TEXT,
    shell_path      TEXT,
    disable_history BOOLEAN NOT NULL DEFAULT 0,
    group_dispatch  BOOLEAN NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL
);

-- Hosts are a sibling of listeners under a project, not a child: a single
-- physical machine may reconnect through many listeners and must still merge
-- to one host. machine_id is NULL when the agent couldn't read a stable
-- platform id; fingerprint (hash of hostname + sorted MACs) is always set
-- and flagged so the UI can surface "fingerprint fallback" on those rows.
CREATE TABLE hosts (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    machine_id           TEXT,
    fingerprint          TEXT NOT NULL,
    fingerprint_fallback BOOLEAN NOT NULL,
    hostname             TEXT,
    primary_alias        TEXT,
    os                   TEXT,
    first_seen_at        DATETIME NOT NULL,
    last_seen_at         DATETIME NOT NULL,
    UNIQUE (project_id, machine_id),
    UNIQUE (project_id, fingerprint)
);

CREATE TABLE sessions (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id),
    listener_id      TEXT NOT NULL REFERENCES listeners(id),
    host_id          TEXT NOT NULL REFERENCES hosts(id),
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

CREATE INDEX idx_sessions_host ON sessions(host_id);
CREATE INDEX idx_sessions_listener ON sessions(listener_id);
CREATE INDEX idx_sessions_live ON sessions(project_id) WHERE disconnected_at IS NULL;
