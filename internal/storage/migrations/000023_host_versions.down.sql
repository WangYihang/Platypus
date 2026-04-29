-- Reverse 000023_host_versions. Restores the legacy agent_version
-- column so operators downgrading to an earlier server release keep a
-- working schema. Best-effort backfill: copy build_version into
-- agent_version where present, on the theory that a binary build
-- identifier is the closest thing the old column ever held.

ALTER TABLE hosts ADD COLUMN agent_version TEXT;

UPDATE hosts SET agent_version = build_version
 WHERE build_version IS NOT NULL AND build_version <> '';

ALTER TABLE hosts DROP COLUMN protocol_version;
ALTER TABLE hosts DROP COLUMN build_date;
ALTER TABLE hosts DROP COLUMN build_commit;
ALTER TABLE hosts DROP COLUMN build_version;
