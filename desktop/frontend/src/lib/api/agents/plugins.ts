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
