// Frontend client for the marketplace catalog endpoints.
// Mirrors the server-side shape in internal/core/plugin/queries.go's
// PluginRow + RefreshStatus. The catalog itself is a SQLite-cached
// mirror of the platypus-plugins index repo; the UI never talks to
// the index directly.

import { authJSON } from "../auth";

/**
 * One plugin version cached from the index. Capabilities are the
 * declared (request) set — what the plugin asks for. The
 * operator-confirmed (granted) set is decided at install time.
 */
export interface MarketplacePlugin {
    plugin_id: string;
    version: string;
    name: string;
    author: string;
    license: string;
    homepage: string;
    description: string;
    latest_version: string;
    publisher_key_id: string;
    wasm_url: string;
    signature_url: string;
    manifest_url?: string;
    wasm_sha256_hex: string;
    capabilities: string[];
    tags?: string[];
    fetched_at_unix: number;
}

export interface MarketplaceRefreshStatus {
    index_url: string;
    last_fetched_unix: number;
    last_status: "ok" | "http_error" | "parse_error" | "fetch_error" | "db_error";
    last_error?: string;
    plugin_count: number;
}

interface PluginsResponse {
    plugins: MarketplacePlugin[];
}

interface VersionsResponse {
    versions: MarketplacePlugin[];
}

interface StatusResponse {
    status: MarketplaceRefreshStatus | "never_synced";
}

interface RefreshResponse {
    plugin_count: number;
}

/**
 * Browse plugins. q is a substring match against the display name —
 * empty/undefined returns the latest of every plugin, sorted by name.
 */
export async function searchPlugins(q?: string): Promise<MarketplacePlugin[]> {
    const url = "/api/v1/marketplace/plugins" + (q ? `?q=${encodeURIComponent(q)}` : "");
    const r = await authJSON<PluginsResponse>(url);
    return r.plugins ?? [];
}

/**
 * Full version history for one plugin id, newest-first.
 */
export async function pluginVersions(pluginID: string): Promise<MarketplacePlugin[]> {
    const r = await authJSON<VersionsResponse>(
        `/api/v1/marketplace/plugins/${encodeURIComponent(pluginID)}/versions`,
    );
    return r.versions ?? [];
}

/**
 * Last-sync row. The literal string `"never_synced"` is returned when
 * the catalog has never been refreshed (fresh deploy, empty index URL).
 */
export async function refreshStatus(): Promise<StatusResponse["status"]> {
    const r = await authJSON<StatusResponse>("/api/v1/marketplace/status");
    return r.status;
}

/**
 * Force-refresh the catalog from the index URL. Admin-only on the
 * server. Returns the number of plugin-version rows landed.
 */
export async function refreshCatalog(): Promise<number> {
    const r = await authJSON<RefreshResponse>("/api/v1/marketplace/refresh", {
        method: "POST",
    });
    return r.plugin_count;
}
