-- 000010_admin_settings.down.sql — revert the admin_settings key/value
-- store. Any overrides in this table are lost on down; the server will
-- fall back to YAML / hardcoded defaults on next start.

DROP TABLE IF EXISTS admin_settings;
