-- 000036_hosts_archive.up.sql — give operators a way to get a host row
-- out of the fleet list.
--
-- A row enters `hosts` via Upsert the first time an agent enrolls and,
-- until now, never left by any path. There was no DELETE route, no
-- Delete on HostRepo, and Reject only moves approval_status to
-- 'rejected' — the row stays in the list either way. So every
-- throwaway agent, every decommissioned box, and every agent an admin
-- explicitly refused accumulated in the operator's fleet view
-- permanently, with no means of clearing them.
--
-- archived_at is a soft delete, deliberately not a hard one. The rows
-- that reference a host are the kind you regret destroying: terminal
-- recordings (rows plus .cast files on disk), security scans, and
-- config audits are the evidence that a given machine was reachable,
-- was scanned, and was non-compliant on a particular date. Hiding the
-- host from a list should not be able to erase that, and an operator
-- tidying a UI is not making a data-retention decision. Archiving is
-- reversible; a cascade is not.
--
-- Reads filter on archived_at IS NULL. Enrollment deliberately does
-- not: an archived host whose agent re-enrolls is un-archived by
-- HostRepo.Upsert, so the row it already has (with its history) is
-- reused rather than a duplicate appearing under a new id.

ALTER TABLE hosts ADD COLUMN archived_at DATETIME;
ALTER TABLE hosts ADD COLUMN archived_by_user TEXT REFERENCES users(id);
ALTER TABLE hosts ADD COLUMN archived_reason TEXT;

-- Every list query in HostRepo is scoped by project and filters
-- archived rows out, so lead with the two columns that narrow first.
-- Partial index on the live rows only: the archived tail is read by id
-- (the restore path) and by the explicitly-archived listing, neither
-- of which needs this index to stay small.
CREATE INDEX idx_hosts_project_active
    ON hosts(project_id, hostname)
    WHERE archived_at IS NULL;
