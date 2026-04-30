-- 000024_security_scans.up.sql — persist host-hardening scan results
-- so the UI can render last-known posture without an agent round-
-- trip, and so cross-host aggregation is a SQL query rather than an
-- N×agent fan-out.
--
-- Two tables, normalised:
--   host_security_scans    one row per scan run
--   host_security_findings one row per finding, FK → scans.id
-- Severity counts live on the parent row so the host-list endpoint
-- (used by HostCard) reads them without joining N findings.
--
-- Retention is application-managed: the storage layer keeps the most
-- recent SCAN_KEEP_PER_HOST scans per host, deleting older ones in
-- the same transaction that inserts the new scan. Findings cascade
-- via the FK below.

CREATE TABLE host_security_scans (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL,
    host_id                TEXT NOT NULL,
    started_at_unix        INTEGER NOT NULL,
    elapsed_ms             INTEGER NOT NULL,
    error                  TEXT NOT NULL DEFAULT '',
    -- Denormalised severity histogram. Cheap to read on the hosts
    -- list endpoint; cheap to maintain (set once at insert).
    finding_count_critical INTEGER NOT NULL DEFAULT 0,
    finding_count_high     INTEGER NOT NULL DEFAULT 0,
    finding_count_medium   INTEGER NOT NULL DEFAULT 0,
    finding_count_low      INTEGER NOT NULL DEFAULT 0,
    finding_count_info     INTEGER NOT NULL DEFAULT 0,
    -- Per-check status snapshot ([{id,category,status,error,elapsed_ms,
    -- finding_count}, ...]). Stored as a JSON blob because it's
    -- read-mostly and the shape is forward-compatible (new check
    -- fields land here without an ALTER).
    checks_json            TEXT NOT NULL DEFAULT '[]',
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

CREATE INDEX idx_security_scans_host_started
    ON host_security_scans(host_id, started_at_unix DESC);
CREATE INDEX idx_security_scans_project_started
    ON host_security_scans(project_id, started_at_unix DESC);

CREATE TABLE host_security_findings (
    id                 TEXT PRIMARY KEY,
    scan_id            TEXT NOT NULL,
    host_id            TEXT NOT NULL,
    project_id         TEXT NOT NULL,
    finding_id         TEXT NOT NULL,   -- e.g. "ssh.permitrootlogin"
    check_id           TEXT NOT NULL,   -- e.g. "ssh"
    category           TEXT NOT NULL,
    severity           TEXT NOT NULL,   -- 'critical'|'high'|'medium'|'low'|'info'
    title              TEXT NOT NULL,
    description        TEXT NOT NULL,
    evidence           TEXT NOT NULL,
    remediation        TEXT NOT NULL,
    references_json    TEXT NOT NULL DEFAULT '[]',
    scanned_at_unix    INTEGER NOT NULL,
    FOREIGN KEY (scan_id) REFERENCES host_security_scans(id) ON DELETE CASCADE,
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

CREATE INDEX idx_security_findings_project_severity
    ON host_security_findings(project_id, severity);
CREATE INDEX idx_security_findings_host_scan
    ON host_security_findings(host_id, scan_id);
CREATE INDEX idx_security_findings_project_category
    ON host_security_findings(project_id, category);
