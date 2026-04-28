-- 000019_rbac.up.sql — replace the hard-coded role enum on
-- users.role / project_members.role with a real RBAC model: a
-- permission catalogue, a roles table that names a permission set,
-- and a role_permissions join.
--
-- The string values currently held in users.role and
-- project_members.role ('admin', 'operator', 'viewer') line up
-- 1-to-1 with the slugs of the three builtin roles seeded below,
-- so this migration adds the RBAC tables and seeds the catalogue
-- but does NOT yet swap the FK on users / project_members. That
-- swap lands in migration 19, after the application code is ready
-- to read from the new tables.

CREATE TABLE permissions (
    slug        TEXT PRIMARY KEY,
    -- resource is the UI grouping ("hosts", "files", "admin"). It's
    -- not part of any access decision — admin pages render
    -- permissions grouped by resource so a role editor can scan a
    -- short list per box rather than a flat 17-item table.
    resource    TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at  DATETIME NOT NULL
);

CREATE TABLE roles (
    slug        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    -- is_builtin: viewer / operator / admin. Cannot be deleted or
    -- renamed; permission set is editable so an operator can add
    -- enrollment:issue to viewer without forking a custom role.
    is_builtin  INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0,1)),
    -- Slot affinity. is_global=1 means this role can be assigned
    -- to users.role; is_project=1 means it can sit in
    -- project_members.role. At least one must be 1 — a role with
    -- both 0 cannot be assigned anywhere.
    is_global   INTEGER NOT NULL DEFAULT 1 CHECK (is_global IN (0,1)),
    is_project  INTEGER NOT NULL DEFAULT 1 CHECK (is_project IN (0,1)),
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    CHECK (is_global = 1 OR is_project = 1)
);

CREATE TABLE role_permissions (
    role_slug TEXT NOT NULL REFERENCES roles(slug) ON DELETE CASCADE,
    perm_slug TEXT NOT NULL REFERENCES permissions(slug) ON DELETE CASCADE,
    PRIMARY KEY (role_slug, perm_slug)
);

-- Catalogue. Adding a new permission later = INSERT here + grant
-- it to whatever roles need it. Removing is rare and dangerous
-- (cascades to role_permissions); model the deletion as a
-- migration when it has to happen.
INSERT INTO permissions (slug, resource, description, created_at) VALUES
    ('hosts:read',         'hosts',      'List hosts and read their metadata.',                       CURRENT_TIMESTAMP),
    ('hosts:exec',         'hosts',      'Run shell commands and interactive sessions on hosts.',     CURRENT_TIMESTAMP),
    ('files:read',         'files',      'Read files on managed hosts.',                              CURRENT_TIMESTAMP),
    ('files:write',        'files',      'Upload, edit, and chmod files on managed hosts.',           CURRENT_TIMESTAMP),
    ('files:delete',       'files',      'Delete files and directories on managed hosts.',            CURRENT_TIMESTAMP),
    ('projects:read',      'projects',   'List projects and view project metadata.',                  CURRENT_TIMESTAMP),
    ('projects:create',    'projects',   'Create new projects.',                                      CURRENT_TIMESTAMP),
    ('projects:settings',  'projects',   'Edit project settings (name, members, defaults).',          CURRENT_TIMESTAMP),
    ('projects:delete',    'projects',   'Delete projects.',                                          CURRENT_TIMESTAMP),
    ('activity:read',      'activity',   'Read the activities / audit timeline.',                     CURRENT_TIMESTAMP),
    ('activity:export',    'activity',   'Export activities to CSV / NDJSON.',                        CURRENT_TIMESTAMP),
    ('enrollment:issue',   'enrollment', 'Issue agent enrollment tokens (plt_*) and install links.', CURRENT_TIMESTAMP),
    ('enrollment:revoke',  'enrollment', 'Revoke agent enrollment tokens before they are redeemed.',  CURRENT_TIMESTAMP),
    ('rpc:invoke',         'rpc',        'Invoke arbitrary agent RPCs (programmatic interface).',     CURRENT_TIMESTAMP),
    ('admin:users',        'admin',      'Create, edit, and delete user accounts.',                   CURRENT_TIMESTAMP),
    ('admin:roles',        'admin',      'Create, edit, and delete RBAC roles.',                      CURRENT_TIMESTAMP),
    ('admin:settings',     'admin',      'Edit server-wide settings.',                                CURRENT_TIMESTAMP);

-- Builtin roles. The slugs (viewer / operator / admin) match the
-- string values held today in users.role / project_members.role,
-- so the migration that swaps those columns to FKs (000019) is a
-- pure schema rewire, not a data move.
INSERT INTO roles (slug, name, description, is_builtin, is_global, is_project, created_at, updated_at) VALUES
    ('viewer',
     'Viewer',
     'Read-only across hosts, files, projects, and activity. Cannot run commands or alter state.',
     1, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('operator',
     'Operator',
     'Day-to-day operations: read, exec, file CRUD, RPC, and enrollment-token lifecycle.',
     1, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('admin',
     'Admin',
     'Full access including user / role management and server settings.',
     1, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT INTO role_permissions (role_slug, perm_slug) VALUES
    -- viewer: read-only.
    ('viewer', 'hosts:read'),
    ('viewer', 'files:read'),
    ('viewer', 'projects:read'),
    ('viewer', 'activity:read'),
    -- operator: viewer + write.
    ('operator', 'hosts:read'),
    ('operator', 'files:read'),
    ('operator', 'projects:read'),
    ('operator', 'activity:read'),
    ('operator', 'hosts:exec'),
    ('operator', 'files:write'),
    ('operator', 'files:delete'),
    ('operator', 'rpc:invoke'),
    ('operator', 'enrollment:issue'),
    ('operator', 'enrollment:revoke'),
    -- admin: every permission, including admin:*.
    ('admin', 'hosts:read'),
    ('admin', 'hosts:exec'),
    ('admin', 'files:read'),
    ('admin', 'files:write'),
    ('admin', 'files:delete'),
    ('admin', 'projects:read'),
    ('admin', 'projects:create'),
    ('admin', 'projects:settings'),
    ('admin', 'projects:delete'),
    ('admin', 'activity:read'),
    ('admin', 'activity:export'),
    ('admin', 'enrollment:issue'),
    ('admin', 'enrollment:revoke'),
    ('admin', 'rpc:invoke'),
    ('admin', 'admin:users'),
    ('admin', 'admin:roles'),
    ('admin', 'admin:settings');

-- Self-protect: the admin role's three admin:* permissions cannot
-- be unlinked. UI is the first line of defense (the checkboxes for
-- those rows are disabled), but a raw DELETE / direct SQL would
-- otherwise let an actor strip the very permissions they used to
-- reach this table — including admin:roles, the permission that
-- gates this surface. Triggers fire AFTER constraint checks but
-- BEFORE the row is actually removed; RAISE(ABORT) leaves the row
-- in place and surfaces a clear error to the caller.
CREATE TRIGGER trg_protect_admin_role_perms
BEFORE DELETE ON role_permissions
WHEN OLD.role_slug = 'admin'
 AND OLD.perm_slug IN ('admin:users', 'admin:roles', 'admin:settings')
BEGIN
    SELECT RAISE(ABORT, 'admin role cannot lose admin:* permissions');
END;

-- Index for the hot RBAC path: "does role X have permission Y?".
-- The composite primary key on (role_slug, perm_slug) already
-- supports this lookup; no extra index needed.

-- Swap users.role and project_members.role from a hard-coded CHECK
-- enum to a real FK to roles.slug.
--
-- The string values currently held in those columns ('admin' /
-- 'operator' / 'viewer') match the slugs of the three builtin roles
-- seeded above, so the SELECT into the rebuilt tables doesn't move
-- a single byte of data — it just re-tags the column with its new
-- constraint. SQLite ≥ 3.26 (which the modernc.org/sqlite driver
-- ships) automatically rewrites cross-table FKs (refresh_tokens,
-- auth_tokens, projects, project_members → users and project_members
-- → projects) when the parent table is dropped+renamed inside one
-- transaction with foreign_keys=ON; migration 16's
-- migration_rename_enrollment_test pinned that behaviour for the
-- enrollment-token rename and the same guarantee carries through
-- here. A migration_rbac_users_fk test asserts foreign_key_check
-- stays clean after this migration runs.

CREATE TABLE users_new (
    id              TEXT PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL REFERENCES roles(slug),
    created_at      DATETIME NOT NULL,
    last_login_at   DATETIME
);
INSERT INTO users_new SELECT id, username, password_hash, role, created_at, last_login_at FROM users;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

CREATE TABLE project_members_new (
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL REFERENCES roles(slug),
    PRIMARY KEY (project_id, user_id)
);
INSERT INTO project_members_new SELECT project_id, user_id, role FROM project_members;
DROP TABLE project_members;
ALTER TABLE project_members_new RENAME TO project_members;
