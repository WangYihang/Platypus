-- Richer machine classification + chassis / GPU summary fields. These
-- are denormalised onto the hosts row (alongside the OS / CPU / memory
-- columns from 000011) so the fleet list can show a machine-type icon
-- without issuing a live SysInfo RPC. All columns are nullable — older
-- agents and locked-down hosts may not report every value, and the UI
-- renders NULL as "—" rather than an empty string or a zero.
ALTER TABLE hosts ADD COLUMN machine_type    TEXT;
ALTER TABLE hosts ADD COLUMN chassis_type    TEXT;
ALTER TABLE hosts ADD COLUMN product_vendor  TEXT;
ALTER TABLE hosts ADD COLUMN product_name    TEXT;
ALTER TABLE hosts ADD COLUMN bios_vendor     TEXT;
ALTER TABLE hosts ADD COLUMN bios_version    TEXT;
-- gpu_summary is a short "vendor model; vendor model" string built
-- server-side from the first few GPU entries. The full, live GPU list
-- stays in-RPC only — utilization / VRAM-used change too often to
-- make caching worth the write amplification.
ALTER TABLE hosts ADD COLUMN gpu_summary     TEXT;
