import { authJSON } from "../auth";

// Build metadata + live counts. Backed by /api/v1/info, intended for
// low-frequency status-bar polling.
export interface ServerInfo {
    version: string;
    commit: string;
    date: string;
    git_repo?: string;

    started_at: string;
    started_at_unix?: number;

    goroutines?: number;
    mem_alloc_bytes?: number;
    /** Process CPU%, per-core normalised by gopsutil. Values above
     *  100 mean multi-core busy. */
    cpu_percent?: number;

    public_addr: string;

    // Counts. session_count is the legacy "live agent registry" value;
    // newer code reads live_session_count / total_session_count (DB
    // ground truth) and host_count / live_host_count.
    session_count: number;
    live_session_count?: number;
    total_session_count?: number;
    host_count?: number;
    live_host_count?: number;
}

export async function getServerInfo(): Promise<ServerInfo> {
    return authJSON<ServerInfo>("/api/v1/info");
}
