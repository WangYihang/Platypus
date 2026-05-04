// Frontend client for the per-agent plugin REST endpoints. Mirrors
// the server-side shapes in internal/api/handler_plugins_*.go.
//
// All endpoints are project-admin-gated. They translate HTTP into a
// PluginMgmtRequest streamed against the live agent link, so the
// agent must be connected at request time — `agent not connected`
// surfaces as a 404.

import { authJSON } from "../../auth";

/** One installed plugin on the agent. Mirrors PluginInfoJSON. */
export interface InstalledPlugin {
    id: string;
    name: string;
    version: string;
    author: string;
    enabled: boolean;
    granted_capabilities: string[];
    install_unix: number;
    source_url?: string;
    publisher_key_id: string;
}

/** One log entry from the agent's per-plugin in-memory ring. */
export interface PluginLogEntry {
    unix_nano: number;
    level: string;
    message: string;
    correlation_id?: string;
}

interface ListResponse {
    plugins: InstalledPlugin[];
}

interface LogsResponse {
    entries: PluginLogEntry[];
}

interface OkResponse {
    status: string;
    plugin_id: string;
    enabled?: boolean;
}

/**
 * List installed plugins on the given agent. Returns [] when the agent
 * has none; throws when the agent is offline (404 from the server).
 */
export async function listPlugins(
    pid: string,
    agentID: string,
): Promise<InstalledPlugin[]> {
    const r = await authJSON<ListResponse>(
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins`,
    );
    return r.plugins ?? [];
}

/**
 * Toggle a plugin's enabled flag. Disabled plugins stay loaded
 * (cheap) but every Invoke against them returns "plugin disabled"
 * without entering the wasm runtime.
 */
export async function enablePlugin(
    pid: string,
    agentID: string,
    pluginID: string,
    enabled: boolean,
): Promise<void> {
    await authJSON<OkResponse>(
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins/${encodeURIComponent(pluginID)}`,
        {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ enabled }),
        },
    );
}

export interface UninstallOptions {
    /** Wipe the plugin's state/ subdir too. Default keeps it for
     *  reinstall. */
    purgeState?: boolean;
}

/**
 * Uninstall a plugin. The agent removes the catalog entry + on-disk
 * install dir. If purgeState is true the plugin's state/ subdir
 * (host_kv_* backing store) is also dropped.
 */
export async function uninstallPlugin(
    pid: string,
    agentID: string,
    pluginID: string,
    opts: UninstallOptions = {},
): Promise<void> {
    await authJSON<OkResponse>(
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins/${encodeURIComponent(pluginID)}`,
        {
            method: "DELETE",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ purge_state: !!opts.purgeState }),
        },
    );
}

/** One progress phase emitted by the agent during install. */
export interface InstallProgress {
    phase: string;
    bytes_done?: number;
    bytes_total?: number;
    error_code?: string;
    error_message?: string;
}

/** REST response shape from the install endpoints. Mirrors the Go
 *  installResponse struct. */
export interface InstallResult {
    status: "installed" | "failed" | "in_progress";
    plugin_id: string;
    version: string;
    progress: InstallProgress[];
}

export interface InstallFromMarketplaceArgs {
    pluginID: string;
    version: string;
    grantedCapabilities: string[];
}

/**
 * Install a plugin from the marketplace catalog. Server fetches the
 * artefacts (no CORS), verifies sha256, then streams into the agent's
 * install pipeline. Returns the full progress array — last entry's
 * phase is INSTALLED on the happy path.
 *
 * 503 = catalog/marketplace not configured on the server.
 * 424 = catalog row exists but has no publisher pubkey (sync needed).
 * 404 = no row matching plugin_id+version.
 */
export async function installFromMarketplace(
    pid: string,
    agentID: string,
    args: InstallFromMarketplaceArgs,
): Promise<InstallResult> {
    return authJSON<InstallResult>(
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins/install_marketplace`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                plugin_id: args.pluginID,
                version: args.version,
                granted_capabilities: args.grantedCapabilities,
            }),
        },
    );
}

/**
 * Install a SYSTEM plugin from the server's local catalog under
 * <data-dir>/system-plugins/. Distinct from the marketplace path
 * because system plugins are signed by the system publisher key
 * and live on the server's local disk (the publisher writes them
 * there at compose-up; production seeds them out-of-band). Same
 * agent-side install pipeline — verify_sig, sha256, load — so the
 * progress array shape matches.
 *
 * 503 = WithSystemBundle was not called on the handler (no data-dir).
 * 404 = the plugin / version isn't staged on this server.
 * 424 = system bundle is missing publisher.pub at its root.
 */
export async function installFromSystem(
    pid: string,
    agentID: string,
    args: InstallFromMarketplaceArgs,
): Promise<InstallResult> {
    return authJSON<InstallResult>(
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins/install_system`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                plugin_id: args.pluginID,
                version: args.version,
                granted_capabilities: args.grantedCapabilities,
            }),
        },
    );
}

/**
 * Tail the most recent N log entries from the agent's in-memory ring
 * for one plugin. tail=0 returns whatever's currently buffered (cap
 * decided agent-side).
 */
export async function pluginLogs(
    pid: string,
    agentID: string,
    pluginID: string,
    tail = 0,
): Promise<PluginLogEntry[]> {
    const url =
        `/api/v1/projects/${encodeURIComponent(pid)}/agents/${encodeURIComponent(agentID)}/plugins/${encodeURIComponent(pluginID)}/logs` +
        (tail > 0 ? `?tail=${tail}` : "");
    const r = await authJSON<LogsResponse>(url);
    return r.entries ?? [];
}
