-- 000026_config_audits.sql — persist host configuration-audit
-- (sensitive-info leak detection) results, parallel to security scans
-- but kept in independent tables so the two audit families can evolve
-- without disturbing each other.
--
--   host_config_audits  one row per audit run
--   host_config_leaks   one row per leak, FK → audits.id
--
-- Risk counts live on the parent row so the host-list endpoint can
-- render a "leak-count" badge without joining N rows. Retention is
-- application-managed (keep most-recent N per host); leaks cascade
-- via the FK below.

CREATE TABLE host_config_audits (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    host_id             TEXT NOT NULL,
    started_at_unix     INTEGER NOT NULL,
    elapsed_ms          INTEGER NOT NULL,
    error               TEXT NOT NULL DEFAULT '',
    -- Denormalised risk histogram. The UI's host card reads these
    -- without ever joining the leaks table.
    leak_count_high     INTEGER NOT NULL DEFAULT 0,
    leak_count_medium   INTEGER NOT NULL DEFAULT 0,
    leak_count_low      INTEGER NOT NULL DEFAULT 0,
    leak_count_info     INTEGER NOT NULL DEFAULT 0,
    -- Per-auditor status snapshot ([{id,category,status,error,
    -- elapsed_ms,leak_count}, ...]). Stored as a JSON blob so
    -- forward-compatible additions land without an ALTER.
    auditors_json       TEXT NOT NULL DEFAULT '[]',
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

CREATE INDEX idx_config_audits_host_started
    ON host_config_audits(host_id, started_at_unix DESC);
CREATE INDEX idx_config_audits_project_started
    ON host_config_audits(project_id, started_at_unix DESC);

CREATE TABLE host_config_leaks (
    id                 TEXT PRIMARY KEY,
    audit_id           TEXT NOT NULL,
    host_id            TEXT NOT NULL,
    project_id         TEXT NOT NULL,
    leak_id            TEXT NOT NULL,    -- e.g. "shell.history.gitleaks.aws-access-token"
    auditor_id         TEXT NOT NULL,    -- e.g. "shell.history"
    category           TEXT NOT NULL,    -- env|shell|cloud|database|webapp|ssh
    risk               TEXT NOT NULL,    -- 'high'|'medium'|'low'|'info'
    title              TEXT NOT NULL,
    location           TEXT NOT NULL,    -- "/root/.bash_history:142"
    -- The match column is the redacted snippet. Plaintext credentials
    -- are NEVER stored — agents apply RedactSecret before the value
    -- leaves the agent process and the server trusts that invariant.
    match_redacted     TEXT NOT NULL,
    pattern            TEXT NOT NULL,    -- gitleaks RuleID or "behavior:<id>"
    description        TEXT NOT NULL,
    remediation        TEXT NOT NULL,
    references_json    TEXT NOT NULL DEFAULT '[]',
    scanned_at_unix    INTEGER NOT NULL,
    FOREIGN KEY (audit_id) REFERENCES host_config_audits(id) ON DELETE CASCADE,
    FOREIGN KEY (host_id)  REFERENCES hosts(id)               ON DELETE CASCADE
);

CREATE INDEX idx_config_leaks_project_risk
    ON host_config_leaks(project_id, risk);
CREATE INDEX idx_config_leaks_host_audit
    ON host_config_leaks(host_id, audit_id);
CREATE INDEX idx_config_leaks_project_category
    ON host_config_leaks(project_id, category);
