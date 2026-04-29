import { authJSON } from "../auth";

// Mirrors core.TopologySnapshot. `machines` is the compound-parent
// layer (host + sysinfo + sessions); `mesh_nodes` cross-references
// mesh NodeIDs to their machine; `links` is the undirected edge set
// with traffic counters.

export interface TopologySysInfo {
    kernel_version?: string;
    os_distribution?: string;
    platform?: string;
    platform_version?: string;
    cpu_percent?: number;
    mem_percent?: number;
    mem_total_bytes?: number;
    mem_used_bytes?: number;
    uptime_seconds?: number;
    sampled_at_unix?: number;
}

export interface TopologySession {
    id: string;
    hash?: string;
    user?: string;
    remote_addr?: string;
    version?: string;
    connected_at: string;
    disconnected_at?: string;
    mesh_node_id?: string;
    active: boolean;
}

export interface TopologyMachine {
    host_id: string;
    project_id: string;
    hostname?: string;
    machine_id?: string;
    os?: string;
    fingerprint: string;
    first_seen_at: string;
    last_seen_at: string;
    sys_info?: TopologySysInfo;
    sessions: TopologySession[];
}

export interface TopologyMeshNodeRef {
    node_id: string;
    kind: "self" | "agent" | "unknown";
    host_id?: string;
    project_id?: string;
}

export interface TopologyLink {
    a: string;
    b: string;
    up: boolean;
    rtt_ns?: number;
    bytes_in: number;
    bytes_out: number;
    msgs_in: number;
    msgs_out: number;
    since?: string;
}

export interface TopologySnapshot {
    generated_at: string;
    project_id: string;
    mesh_enabled: boolean;
    machines: TopologyMachine[];
    mesh_nodes: TopologyMeshNodeRef[];
    links: TopologyLink[];
}

export async function fetchTopologySnapshot(pid: string): Promise<TopologySnapshot> {
    return authJSON<TopologySnapshot>(`/api/v1/projects/${pid}/topology`);
}

export interface LinkHistoryPoint {
    at: string;
    bytes_in: number;
    bytes_out: number;
    msgs_in: number;
    msgs_out: number;
    rtt_ns?: number;
}

export interface MachineHistoryPoint {
    at: string;
    cpu_percent?: number;
    mem_percent?: number;
}

export interface HistoryOpts {
    since?: Date;
    until?: Date;
    max?: number;
}

function buildHistoryParams(opts: HistoryOpts): string {
    const p = new URLSearchParams();
    if (opts.since) p.set("since", opts.since.toISOString());
    if (opts.until) p.set("until", opts.until.toISOString());
    if (opts.max && opts.max > 0) p.set("max", String(opts.max));
    const s = p.toString();
    return s ? "?" + s : "";
}

export async function fetchLinkHistory(
    pid: string,
    a: string,
    b: string,
    opts: HistoryOpts = {},
): Promise<LinkHistoryPoint[]> {
    const j = await authJSON<{ points: LinkHistoryPoint[] }>(
        `/api/v1/projects/${pid}/topology/links/${encodeURIComponent(a)}/${encodeURIComponent(b)}/stats${buildHistoryParams(opts)}`,
    );
    return j.points;
}

export async function fetchMachineHistory(
    pid: string,
    hid: string,
    opts: HistoryOpts = {},
): Promise<MachineHistoryPoint[]> {
    const j = await authJSON<{ points: MachineHistoryPoint[] }>(
        `/api/v1/projects/${pid}/topology/machines/${encodeURIComponent(hid)}/stats${buildHistoryParams(opts)}`,
    );
    return j.points;
}
