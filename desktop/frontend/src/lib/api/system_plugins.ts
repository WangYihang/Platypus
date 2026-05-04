// Frontend client for the system-plugins admin endpoint.
//
// System plugins are the agent-side bundle staged by the dev
// publisher (or seeded manually in production) at
// <data-dir>/system-plugins/. Distinct from the marketplace catalog:
//   - signed by the SYSTEM publisher key (not the marketplace key)
//   - eligible to be pre-installed via the install bundle's
//     baseline_plugin_ids (Phase A)
//   - shown in the enroll wizard's BaselinePluginsStep
//
// The endpoint is auth-only (no project role gate) since the
// catalog is server-wide. Returns an empty array when the publisher
// hasn't staged anything; the wizard renders an empty-state hint
// in that case.

import { authJSON } from "../auth";

export interface SystemPlugin {
    id: string;
    name: string;
    version: string;
    description?: string;
    author?: string;
    license?: string;
    capabilities: string[];
    /** v2pb.StreamType strings claimed by this plugin (e.g. STREAM_TYPE_FILE_READ). */
    streams?: string[];
}

interface ListResponse {
    plugins: SystemPlugin[];
}

/**
 * listSystemPlugins fetches the server's system-plugin catalog.
 * Empty array on a fresh deployment that hasn't run the publisher;
 * caller should render an empty-state hint in that case.
 */
export async function listSystemPlugins(): Promise<SystemPlugin[]> {
    const r = await authJSON<ListResponse>("/api/v1/system-plugins");
    return r.plugins ?? [];
}
