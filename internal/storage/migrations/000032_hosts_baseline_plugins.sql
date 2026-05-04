-- The operator's system-plugin allowlist now lives on the host
-- record (server-side source of truth) once the install token is
-- consumed. The link handler reconciles each agent's installed
-- catalog against this column on every connect, pushing missing
-- plugins from <data_dir>/system-plugins/ via the existing PluginMgmt
-- install_system stream.
--
-- Comma-separated string, matching the convention from migration
-- 000031 on install_download_tokens. Empty string = "operator picked
-- nothing"; the link reconciler still adds the mandatory core
-- (sys-info) on top so host overview is never blank.
--
-- install_download_tokens.baseline_plugin_ids stays in place as the
-- mint→consume carrier; it gets copied here at consume time, then
-- the install token row is effectively dead.
ALTER TABLE hosts
    ADD COLUMN baseline_plugin_ids TEXT NOT NULL DEFAULT '';
