-- 000005_project_ca.up.sql — per-project Certificate Authority + every
-- agent identity certificate it issues. Append-only for the same
-- reasons as the PAT / session tables: revocation marks a row, never
-- deletes it, so historical state remains queryable.
--
-- The CA private key is stored encrypted by an operator-supplied KEK
-- (PLATYPUS_CA_KEK env var — hex-encoded 32 bytes for AES-256-GCM).
-- This moves the "if the DB leaks, can attackers forge certs?" question
-- from "yes immediately" to "only if they also compromise the KEK."
-- Future work: HSM / cloud KMS wrapping.

CREATE TABLE project_ca (
    project_id      TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    cert_pem        TEXT NOT NULL,           -- self-signed Ed25519 root, PEM
    privkey_nonce   BLOB NOT NULL,           -- 12-byte AES-GCM nonce
    privkey_ct      BLOB NOT NULL,           -- AES-GCM(pkcs8-encoded Ed25519 priv)
    serial_counter  INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL,
    created_by_user TEXT NOT NULL REFERENCES users(id),
    -- Rotation: a future row with revoked_at set flags the old CA as
    -- superseded. For MVP we only ever have one CA per project, but
    -- keeping the column paves the rotation path without a migration.
    revoked_at      DATETIME,
    revoked_by_user TEXT REFERENCES users(id),
    revoked_reason  TEXT,
    CHECK (serial_counter >= 0)
);

-- Every certificate ever issued. `serial` is a per-project monotonic
-- counter incremented atomically at issuance time — combined with the
-- CA's SubjectKeyIdentifier this uniquely identifies a cert without
-- needing a globally unique serial scheme.
CREATE TABLE issued_certs (
    serial          INTEGER NOT NULL,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_id        TEXT,                    -- NULL on initial enroll before agent_id was synthesised; set afterwards
    cert_pem        TEXT NOT NULL,
    pubkey_pem      TEXT NOT NULL,           -- the Ed25519 pubkey the cert binds
    issued_at       DATETIME NOT NULL,
    not_before      DATETIME NOT NULL,
    not_after       DATETIME NOT NULL,
    issued_reason   TEXT NOT NULL,           -- 'enroll' | 'rotation' | 'reissue' | 'admin'
    issued_by_user  TEXT,                    -- admin id when issued manually; NULL when auto
    -- Revocation (admin kill, leaked key, etc.). Rows whose revoked_at
    -- is set and whose not_after is still in the future are in scope
    -- for the CRL endpoint.
    revoked_at      DATETIME,
    revoked_by_user TEXT REFERENCES users(id),
    revoked_reason  TEXT,
    PRIMARY KEY (project_id, serial),
    CHECK (not_after > not_before),
    CHECK (revoked_at IS NULL OR revoked_at >= issued_at),
    CHECK (issued_reason IN ('enroll', 'rotation', 'reissue', 'admin'))
);

-- Hot path: "which certs are live right now for agent X"? Use this
-- partial index (the predicate is deterministic — no now()).
CREATE INDEX idx_issued_certs_agent_live
    ON issued_certs(project_id, agent_id)
    WHERE revoked_at IS NULL;

-- CRL generation path: "which certs were revoked for this project?"
CREATE INDEX idx_issued_certs_revoked
    ON issued_certs(project_id, revoked_at)
    WHERE revoked_at IS NOT NULL;
