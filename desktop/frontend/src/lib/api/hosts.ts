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
    primary_ip_info?: RemoteIpInfo;
    primary_mac?: string;
    boot_time_unix?: number;

    // Egress / public IP (migration 000024). egress_ip is what the
    // server saw as the WS upgrade peer (authoritative "where on the
    // internet did this connect from"). public_ip is the agent's own
    // DNS-TXT probe; the two diverge under mesh relay so we keep
    // both.
    egress_ip?: string;
    egress_ip_info?: RemoteIpInfo;
    public_ip?: string;
    public_ip_info?: RemoteIpInfo;

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

    // Latest security-scan summary, ridden onto the hosts list
    // response by a single batched query server-side. `null`
    // (absent on the wire) = host has never been scanned; populated
    // counts (even all-zero) = scanned, possibly clean. The fleet
    // HostCard distinguishes the two visually.
    security_severity_counts?: SeverityCounts | null;
    security_scanned_at_unix?: number;
}

export type Severity = "critical" | "high" | "medium" | "low" | "info";

export interface SeverityCounts {
    critical: number;
    high: number;
    medium: number;
    low: number;
    info: number;
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

export interface RemoteIpInfo {
    ip: string;
    version?: number;
    is_private?: boolean;
    is_loopback?: boolean;
    country?: string;
    province?: string;
    city?: string;
    isp?: string;
}

export interface SessionRow {
    id: string;
    project_id: string;
    ingress_addr: string;
    host_id: string;
    alias?: string;
    user?: string;
    remote_addr?: string;
    remote_info?: RemoteIpInfo;
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

// Ad-hoc enrichment for an IP that didn't ride along on a richer
// payload (e.g. sysinfo's public_ip, mesh link telemetry). The
// server caches lookups in-process, and React Query dedupes by IP
// on the client, so a dashboard rendering N copies of the same IP
// only triggers one round trip.
export async function lookupIpInfo(ip: string): Promise<RemoteIpInfo> {
    const qs = new URLSearchParams({ ip }).toString();
    return authJSON<RemoteIpInfo>(`/api/v1/ipinfo?${qs}`);
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

// AgentUpgradeRequest is the body the trigger endpoint accepts. Both
// fields are optional: target_version="" defers to the channel head,
// channel="" defaults to "stable" server-side.
export interface AgentUpgradeRequest {
    target_version?: string;
    channel?: string;
}

// AgentUpgradeResponse is the synchronous response from the trigger
// endpoint. The server holds the HTTP request open while the agent
// runs the install, then returns the terminal phase + any error.
//
// status:
//   "exited"      — agent reached PHASE_EXITING; supervisor will
//                   restart it under the new binary
//   "failed"      — agent reported PHASE_FAILED (see error_code /
//                   error_message for the kind)
//   "in_progress" — server timed out waiting for terminal frame
//                   (slow link); upgrade may still complete
//   "unknown"     — drainUpgradeProgress saw a non-terminal phase
//                   and gave up (proto-version drift)
export interface AgentUpgradeResponse {
    status: "exited" | "failed" | "in_progress" | "unknown";
    phase: string;
    resolved_version?: string;
    error_code?: string;
    error_message?: string;
    bytes_done?: number;
    bytes_total?: number;
}

// triggerAgentUpgrade kicks off a server-driven upgrade against a
// specific agent. Request is admin-only; the server records both an
// "agent.upgrade.start" and an "agent.upgrade.end" activity row, so
// closing the browser tab mid-flight still leaves a forensic trail.
export async function triggerAgentUpgrade(
    pid: string,
    agentID: string,
    body: AgentUpgradeRequest,
): Promise<AgentUpgradeResponse> {
    return authJSON<AgentUpgradeResponse>(
        `/api/v1/projects/${pid}/agents/${agentID}/upgrade`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        },
    );
}

// --- Security scan -------------------------------------------------

export interface SecurityFinding {
    id: string;
    finding_id: string;
    check_id: string;
    category: string;
    severity: Severity;
    title: string;
    description: string;
    evidence: string;
    remediation: string;
    references?: string[];
    // Present only on the project-level findings endpoint.
    host_id?: string;
    scanned_at_unix?: number;
}

export interface SecurityCheckResult {
    id: string;
    category: string;
    status: "ok" | "skipped" | "error";
    error?: string;
    elapsed_ms: number;
    finding_count: number;
}

export interface HostSecurityScan {
    id: string;
    host_id: string;
    project_id: string;
    started_at_unix: number;
    elapsed_ms: number;
    error?: string;
    severity_counts: SeverityCounts;
    findings: SecurityFinding[];
    checks: SecurityCheckResult[];
}

export interface SecurityScanSummary {
    id: string;
    started_at_unix: number;
    elapsed_ms: number;
    severity_counts: SeverityCounts;
    error?: string;
}

export interface RescanHostOpts {
    check_ids?: string[];
    categories?: string[];
    per_check_timeout_ms?: number;
}

// AvailableCheck describes one registered checker on the agent. The
// UI renders a checklist of these before any scan runs, so users see
// the full set of what *would* be evaluated. `applicable` reflects
// the agent's Applicable() decision at enumeration time — non-
// applicable checks render dimmed (e.g. ssh.config when sshd_config
// is missing).
//
// title / description / references come from the Checker's
// Metadata() and power the Coverage panel — operators see what we
// inspect, what we don't, and how each check maps to standards
// (CIS sections, CVE ids).
export interface AvailableCheck {
    id: string;
    category: string;
    applicable: boolean;
    title?: string;
    description?: string;
    references?: string[];
}

// listAvailableChecks proxies the agent's ListSecurityChecks RPC.
// Returns null when the agent is offline (404) so the UI can fall
// back to deriving the list from the persisted scan's checks[].
export async function listAvailableChecks(
    pid: string,
    hid: string,
): Promise<AvailableCheck[] | null> {
    try {
        const j = await authJSON<{ checks: AvailableCheck[] }>(
            `/api/v1/projects/${pid}/hosts/${hid}/security-checks`,
        );
        return j.checks ?? [];
    } catch (err) {
        if (err instanceof Error && err.message.startsWith("404:")) return null;
        throw err;
    }
}

export interface ListProjectFindingsOpts {
    severity?: Severity[];
    category?: string[];
    host_id?: string;
    q?: string;
    page?: number;
    page_size?: number;
}

export interface ProjectFindingsPage {
    findings: SecurityFinding[];
    total: number;
    page: number;
    page_size: number;
}

// getHostSecurityScan returns the latest persisted scan, or null when
// the host has never been scanned. Distinguishing "never scanned"
// from "scanned, all clean" is part of the UI's job — keep the null
// pathway intact rather than throwing on 404.
//
// scanID, when supplied, picks one historical scan from the
// per-host history dropdown. ErrNotFound on a stranger's scan id is
// the server's responsibility (handler_hosts_v1 enforces it); we
// surface it as null here for the same reason.
export async function getHostSecurityScan(
    pid: string,
    hid: string,
    scanID?: string,
): Promise<HostSecurityScan | null> {
    const qs = scanID ? `?scan_id=${encodeURIComponent(scanID)}` : "";
    try {
        return await authJSON<HostSecurityScan>(
            `/api/v1/projects/${pid}/hosts/${hid}/security-scan${qs}`,
        );
    } catch (err) {
        if (err instanceof Error && err.message.startsWith("404:")) return null;
        throw err;
    }
}

// rescanHost POSTs to the same path. Triggers an agent RPC, persists
// the result server-side, and returns it. Throws on 4xx/5xx —
// callers should surface humanizeError(err) inline rather than
// clobbering the cached read.
export async function rescanHost(
    pid: string,
    hid: string,
    opts: RescanHostOpts = {},
): Promise<HostSecurityScan> {
    return authJSON<HostSecurityScan>(
        `/api/v1/projects/${pid}/hosts/${hid}/security-scan`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(opts),
        },
    );
}

// listHostScans returns the lightweight history rows for the per-host
// History dropdown. limit defaults to 10, capped server-side at 50.
export async function listHostScans(
    pid: string,
    hid: string,
    limit?: number,
): Promise<SecurityScanSummary[]> {
    const qs = limit && limit > 0 ? `?limit=${limit}` : "";
    const j = await authJSON<{ scans: SecurityScanSummary[] }>(
        `/api/v1/projects/${pid}/hosts/${hid}/security-scans${qs}`,
    );
    return j.scans ?? [];
}

// listProjectFindings powers the cross-host Security top-tab. The
// server restricts results to the latest scan per host so the
// project view always shows current posture, not historical noise.
export async function listProjectFindings(
    pid: string,
    opts: ListProjectFindingsOpts = {},
): Promise<ProjectFindingsPage> {
    const params = new URLSearchParams();
    if (opts.severity?.length) params.set("severity", opts.severity.join(","));
    if (opts.category?.length) params.set("category", opts.category.join(","));
    if (opts.host_id) params.set("host_id", opts.host_id);
    if (opts.q) params.set("q", opts.q);
    if (opts.page && opts.page > 0) params.set("page", String(opts.page));
    if (opts.page_size && opts.page_size > 0) {
        params.set("page_size", String(opts.page_size));
    }
    const qs = params.toString();
    const url = `/api/v1/projects/${pid}/security-findings${qs ? "?" + qs : ""}`;
    return authJSON<ProjectFindingsPage>(url);
}
