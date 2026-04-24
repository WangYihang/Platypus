DROP INDEX IF EXISTS idx_hosts_agent_id;

ALTER TABLE hosts DROP COLUMN agent_version;
ALTER TABLE hosts DROP COLUMN boot_time_unix;
ALTER TABLE hosts DROP COLUMN primary_mac;
ALTER TABLE hosts DROP COLUMN primary_ip;
ALTER TABLE hosts DROP COLUMN timezone;
ALTER TABLE hosts DROP COLUMN current_user;
ALTER TABLE hosts DROP COLUMN mem_total_bytes;
ALTER TABLE hosts DROP COLUMN num_cpu;
ALTER TABLE hosts DROP COLUMN cpu_model;
ALTER TABLE hosts DROP COLUMN kernel_version;
ALTER TABLE hosts DROP COLUMN platform_version;
ALTER TABLE hosts DROP COLUMN platform_family;
ALTER TABLE hosts DROP COLUMN platform;
ALTER TABLE hosts DROP COLUMN arch;
ALTER TABLE hosts DROP COLUMN agent_id;
