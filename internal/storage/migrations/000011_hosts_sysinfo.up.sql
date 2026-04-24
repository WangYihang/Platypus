-- Rich system info captured at enrollment and refreshed whenever a
-- connected agent replies to a SysInfo RPC. All columns are nullable
-- because:
--   * historical rows predate the feature;
--   * minimal / sandboxed agents may not be able to report every value.
-- The Web UI renders NULL as "—", not as 0 / empty string.
ALTER TABLE hosts ADD COLUMN agent_id          TEXT;
ALTER TABLE hosts ADD COLUMN arch              TEXT;
ALTER TABLE hosts ADD COLUMN platform          TEXT;
ALTER TABLE hosts ADD COLUMN platform_family   TEXT;
ALTER TABLE hosts ADD COLUMN platform_version  TEXT;
ALTER TABLE hosts ADD COLUMN kernel_version    TEXT;
ALTER TABLE hosts ADD COLUMN cpu_model         TEXT;
ALTER TABLE hosts ADD COLUMN num_cpu           INTEGER;
ALTER TABLE hosts ADD COLUMN mem_total_bytes   INTEGER;
ALTER TABLE hosts ADD COLUMN current_user      TEXT;
ALTER TABLE hosts ADD COLUMN timezone          TEXT;
ALTER TABLE hosts ADD COLUMN primary_ip        TEXT;
ALTER TABLE hosts ADD COLUMN primary_mac       TEXT;
ALTER TABLE hosts ADD COLUMN boot_time_unix    INTEGER;
ALTER TABLE hosts ADD COLUMN agent_version     TEXT;

-- Looking up a host by the agent that reported it is a common
-- operation (the link handler does it every time an agent reconnects).
-- A partial unique index lets us upsert cheaply and prevents two rows
-- from claiming the same agent.
CREATE UNIQUE INDEX IF NOT EXISTS idx_hosts_agent_id ON hosts(agent_id) WHERE agent_id IS NOT NULL;
