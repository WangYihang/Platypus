-- 000030_marketplace_plugin_publisher_pubkey.sql — extends the
-- marketplace catalog so install_marketplace can reach the agent's
-- install path without a second round-trip.
--
-- Until this column lands the install_marketplace handler had no way
-- to feed PublisherPubkey into v2pb.PluginInstallRequest: the catalog
-- only knew the *id*. Storing the raw .pub bytes inline here is a
-- deliberate denormalisation — the alternative is a publishers table
-- with a join on every install, but the catalog is small (hundreds
-- of plugin-versions) and the .pub file is 32-ish bytes, so the
-- duplication cost is rounding error.
--
-- Nullable on purpose: legacy index.json files (and a fresh
-- deployment that hasn't synced yet) won't have it. Code paths that
-- need it must check for empty + emit a clear error so the operator
-- knows to refresh the catalog against an index that exposes the key.

ALTER TABLE marketplace_plugin_versions
    ADD COLUMN publisher_pubkey BLOB NOT NULL DEFAULT x'';
