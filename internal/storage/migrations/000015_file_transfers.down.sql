-- 000015_file_transfers.down.sql — drop the file-transfer task log.
-- All in-flight and historical rows are lost on down.

DROP INDEX IF EXISTS idx_file_transfers_status;
DROP INDEX IF EXISTS idx_file_transfers_host_started;
DROP INDEX IF EXISTS idx_file_transfers_project_started;
DROP TABLE IF EXISTS file_transfers;
