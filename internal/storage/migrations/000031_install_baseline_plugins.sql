-- baseline plugin allowlist propagated from install token through
-- enroll bundle → agent boot. empty string = "no allowlist set"
-- (agent falls back to its mandatory core only). comma-separated
-- because sqlite has no native list type and this matches existing
-- conventions in the codebase.
ALTER TABLE install_download_tokens
    ADD COLUMN baseline_plugin_ids TEXT NOT NULL DEFAULT '';
