-- 000020_file_transfers_wire_bytes.up.sql — split "bytes transferred"
-- into source vs. wire so archive (tar.gz / zip) progress is meaningful.
--
-- Until now `bytes_transferred` stored whatever the sender counted —
-- which for archive downloads is the *compressed* wire byte count, so
-- the ratio against `total_bytes` (the uncompressed pre-scan total)
-- could overshoot 100% and forced the UI to render an indeterminate
-- bar. We now use:
--   * bytes_transferred = uncompressed source bytes processed (the
--     meaningful progress numerator, comparable to total_bytes)
--   * wire_bytes        = bytes written to the HTTP response (the
--     compressed count for archive; equals bytes_transferred for
--     non-archive transfers since no transformation happens)
--
-- compression ratio = wire_bytes / bytes_transferred, average source
-- throughput = bytes_transferred / elapsed — both renderable in the
-- /transfers table.

ALTER TABLE file_transfers ADD COLUMN wire_bytes INTEGER NOT NULL DEFAULT 0;
