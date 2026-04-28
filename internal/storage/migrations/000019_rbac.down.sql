-- Reverse of 000018: restore the hard-coded role-enum CHECK on
-- users.role / project_members.role and drop the RBAC tables.
-- Row data is preserved across the rebuild — the values match the
-- enum either way.

CREATE TABLE users_old (
    id              TEXT PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at      DATETIME NOT NULL,
    last_login_at   DATETIME
);
INSERT INTO users_old SELECT id, username, password_hash, role, created_at, last_login_at FROM users;
DROP TABLE users;
ALTER TABLE users_old RENAME TO users;

CREATE TABLE project_members_old (
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    PRIMARY KEY (project_id, user_id)
);
INSERT INTO project_members_old SELECT project_id, user_id, role FROM project_members;
DROP TABLE project_members;
ALTER TABLE project_members_old RENAME TO project_members;

DROP TRIGGER IF EXISTS trg_protect_admin_role_perms;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS permissions;
