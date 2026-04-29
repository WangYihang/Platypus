import { authFetch, authJSON } from "../auth";

export interface Host {
    id: string;
    project_id: string;
    machine_id?: string;
    fingerprint: string;
    fingerprint_fallback: boolean;
    hostname?: string;
    primary_alias?: string;
    os?: string;
    first_seen_at: string;
    last_seen_at: string;

    // Rich agent-reported system info. All fields optional — older
    // agents or minimal hosts may leave most of them unset.
    agent_id?: string;
    arch?: string;
    platform?: string;
    platform_family?: string;
    platform_version?: string;
    kernel_version?: string;
    cpu_model?: string;
    num_cpu?: number;
    mem_total_bytes?: number;
    current_user?: string;
    timezone?: string;
    primary_ip?: string;
    primary_mac?: string;
    boot_time_unix?: number;

    // Build identity (migration 000023). build_version is semver,
    // build_commit short git SHA, build_date RFC3339. protocol_version
    // is a separate monotonic uint32 used for wire compatibility.
    build_version?: string;
    build_commit?: string;
    build_date?: string;
    protocol_version?: number;

    // Hardware / chassis classification (migration 000012).
    machine_type?: string;
    chassis_type?: string;
    product_vendor?: string;
    product_name?: string;
    bios_vendor?: string;
    bios_version?: string;
    gpu_summary?: string;

    // Approval gate (migration 000022). Every fresh enrollment lands
    // in `pending` unless the redeemed PAT carried auto_approve.
    approval_status: "pending" | "approved" | "rejected";
    approval_decided_at?: string;
    approval_decided_by?: string;
    approval_reason?: string;
}

export interface HostSysInfoInterface {
    name: string;
    mac?: string;
    addrs?: string[];
    flags?: string;
    mtu?: number;
    is_up?: boolean;
    is_loopback?: boolean;
}

export interface HostSysInfoDisk {
    device?: string;
    mountpoint?: string;
    fstype?: string;
    total_bytes?: number;
    used_bytes?: number;
}

export interface HostSysInfoUser {
    user?: string;
    terminal?: string;
    host?: string;
    started_at?: number;
}

export interface HostSysInfoGPU {
    vendor?: string;
    model?: string;
    driver?: string;
    driver_version?: string;
    vram_total_bytes?: number;
    vram_used_bytes?: number;
    utilization_pct?: number;
    bus_id?: string;
    uuid?: string;
    index?: number;
}

// Live snapshot from the agent's SysInfo RPC. Distinct from `Host`
// (DB-cached) because it includes dynamic metrics like CPU% and load.
export interface HostSysInfo {
    os?: string;
    arch?: string;
    hostname?: string;
    kernel_version?: string;
    cpu_percent?: number;
    mem_total?: number;
    mem_used?: number;
    mem_available?: number;
    mem_free?: number;
    swap_total?: number;
    swap_used?: number;
    disk_total?: number;
    disk_used?: number;
    platform?: string;
    platform_family?: string;
    platform_version?: string;
    virtualization?: string;
    timezone?: string;
    num_cpu?: number;
    num_cpu_physical?: number;
    cpu_model?: string;
    cpu_mhz?: number;
    boot_time_unix?: number;
    uptime_seconds?: number;
    load1?: number;
    load5?: number;
    load15?: number;
    process_count?: number;
    current_user?: string;
    build_version?: string;
    build_commit?: string;
    build_date?: string;
    protocol_version?: number;
    machine_id?: string;
    default_gateway?: string;
    primary_ip?: string;
    primary_mac?: string;
    public_ip?: string;
    interfaces?: HostSysInfoInterface[];
    disks?: HostSysInfoDisk[];
    users?: HostSysInfoUser[];
    sampled_at_unix?: number;
    error?: string;

    machine_type?: string;
    chassis_type?: string;
    product_vendor?: string;
    product_name?: string;
    bios_vendor?: string;
    bios_version?: string;
    container_runtime?: string;
    gpus?: HostSysInfoGPU[];
}

export interface HostProcess {
    pid: number;
    ppid?: number;
    user?: string;
    name?: string;
    cmdline?: string;
    status?: string;
    cpu_percent?: number;
    mem_percent?: number;
    rss_bytes?: number;
    num_threads?: number;
    created_at_unix?: number;
}

export interface HostProcessList {
    processes?: HostProcess[];
    total_count?: number;
    error?: string;
}

export interface SessionRow {
    id: string;
    project_id: string;
    ingress_addr: string;
    host_id: string;
    alias?: string;
    user?: string;
    remote_addr?: string;
    version?: string;
    group_dispatch: boolean;
    connected_at: string;
    disconnected_at?: string;
}

export async function listHosts(pid: string): Promise<Host[]> {
    const j = await authJSON<{ hosts: Host[] }>(`/api/v1/projects/${pid}/hosts`);
    return j.hosts;
}

// Hosts in the project still awaiting admin approval, oldest first.
export async function listPendingApprovals(pid: string): Promise<Host[]> {
    const j = await authJSON<{ hosts: Host[] }>(`/api/v1/projects/${pid}/hosts/pending`);
    return j.hosts;
}

// Cheap COUNT(*) for the top-bar badge — single integer instead of full host rows.
export async function pendingApprovalCount(pid: string): Promise<number> {
    const j = await authJSON<{ pending: number }>(
        `/api/v1/projects/${pid}/hosts/pending/count`,
    );
    return j.pending;
}

export async function approveHost(pid: string, hid: string, reason: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/hosts/${hid}/approve`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason }),
    });
}

export async function rejectHost(pid: string, hid: string, reason: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/hosts/${hid}/reject`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason }),
    });
}

export async function getHost(pid: string, hid: string): Promise<Host> {
    return authJSON<Host>(`/api/v1/projects/${pid}/hosts/${hid}`);
}

// Server proxies the agent's SysInfo RPC. 404 when the agent is offline.
export async function getHostSysInfo(pid: string, hid: string): Promise<HostSysInfo> {
    return authJSON<HostSysInfo>(`/api/v1/projects/${pid}/hosts/${hid}/sysinfo`);
}

export interface ListHostProcessesOpts {
    top?: number;
    sort?: "cpu" | "mem" | "rss" | "pid";
}

// top=0 means "as many as the server cap allows" (500). 404 when offline.
export async function listHostProcesses(
    pid: string,
    hid: string,
    opts: ListHostProcessesOpts = {},
): Promise<HostProcessList> {
    const params = new URLSearchParams();
    if (opts.top != null) params.set("top", String(opts.top));
    if (opts.sort) params.set("sort", opts.sort);
    const qs = params.toString();
    const url = `/api/v1/projects/${pid}/hosts/${hid}/processes${qs ? `?${qs}` : ""}`;
    return authJSON<HostProcessList>(url);
}

export async function listHostSessions(pid: string, hid: string): Promise<SessionRow[]> {
    const j = await authJSON<{ sessions: SessionRow[] }>(
        `/api/v1/projects/${pid}/hosts/${hid}/sessions`,
    );
    return j.sessions;
}

export interface SessionListOpts {
    live?: boolean;
    since?: Date;
    limit?: number;
}

export async function listProjectSessions(
    pid: string,
    opts: SessionListOpts = {},
): Promise<SessionRow[]> {
    const params = new URLSearchParams();
    if (opts.live !== undefined) params.set("live", String(opts.live));
    if (opts.since) params.set("since", opts.since.toISOString());
    if (opts.limit && opts.limit > 0) params.set("limit", String(opts.limit));
    const qs = params.toString();
    const path = `/api/v1/projects/${pid}/sessions${qs ? "?" + qs : ""}`;
    const j = await authJSON<{ sessions: SessionRow[] }>(path);
    return j.sessions;
}
