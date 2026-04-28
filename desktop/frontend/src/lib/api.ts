// Typed wrappers over the /api/v1 surface introduced by the redesign
// (Projects, Hosts, Sessions). Thin on purpose — each
// function is one authJSON call. Response shapes mirror the Go
// handler_*_v2.go structs so a rename on either side will surface at
// compile time.

import { authFetch, authJSON } from "./auth";

// --- Types (mirror handler response shapes) ---------------------------

export interface Project {
    id: string;
    name: string;
    slug: string;
    created_at: string;
    created_by: string;
}

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
    agent_version?: string;

    // Hardware / chassis classification (migration 000012). All
    // optional; machine_type is the coarse category (container / vm /
    // bare_metal / laptop / desktop / unknown). gpu_summary is a
    // short "NVIDIA RTX 4090; Intel UHD 770" blurb for list rendering.
    machine_type?: string;
    chassis_type?: string;
    product_vendor?: string;
    product_name?: string;
    bios_vendor?: string;
    bios_version?: string;
    gpu_summary?: string;
}

// HostSysInfo mirrors v2pb.SysInfoResponse — a full, live snapshot
// the agent returns on demand. Populated by getHostSysInfo; unlike
// `Host` (DB-cached), this includes the dynamic metrics (CPU %,
// memory used, load average, etc.).
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

// HostProcess mirrors v2pb.ProcessInfo. Populated by listHostProcesses
// (live RPC; never cached on the server). rss_bytes is resident-set
// size; mem_percent is gopsutil's calculation (RSS / total RAM).
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
    agent_version?: string;
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

export interface ProjectMember {
    user_id: string;
    username: string;
    role: "admin" | "operator" | "viewer";
}

// --- Projects --------------------------------------------------------

export async function listProjects(): Promise<Project[]> {
    const j = await authJSON<{ projects: Project[] }>("/api/v1/projects");
    return j.projects;
}

export async function createProject(name: string, slug: string): Promise<Project> {
    return authJSON<Project>("/api/v1/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, slug }),
    });
}

export async function deleteProject(pid: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}`, { method: "DELETE" });
}

export async function listProjectMembers(pid: string): Promise<ProjectMember[]> {
    const j = await authJSON<{ members: ProjectMember[] }>(`/api/v1/projects/${pid}/members`);
    return j.members;
}

export async function addProjectMember(
    pid: string,
    userID: string,
    role: ProjectMember["role"],
): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/members`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userID, role }),
    });
}

export async function removeProjectMember(pid: string, userID: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/members/${userID}`, { method: "DELETE" });
}

// --- Hosts -----------------------------------------------------------

export async function listHosts(pid: string): Promise<Host[]> {
    const j = await authJSON<{ hosts: Host[] }>(`/api/v1/projects/${pid}/hosts`);
    return j.hosts;
}

export async function getHost(pid: string, hid: string): Promise<Host> {
    return authJSON<Host>(`/api/v1/projects/${pid}/hosts/${hid}`);
}

// getHostSysInfo asks the server to call the agent's SysInfo RPC
// and forward the raw response. Expected to 404 when the agent is
// offline; callers should handle that gracefully (show cached
// static fields from Host instead).
export async function getHostSysInfo(pid: string, hid: string): Promise<HostSysInfo> {
    return authJSON<HostSysInfo>(`/api/v1/projects/${pid}/hosts/${hid}/sysinfo`);
}

// listHostProcesses proxies the agent's ProcessList RPC. Same 404
// semantics as getHostSysInfo when the agent is offline. top=0 means
// "as many as the server cap allows" (500).
export interface ListHostProcessesOpts {
    top?: number;
    sort?: "cpu" | "mem" | "rss" | "pid";
}
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

// listProjectSessions returns sessions across the whole project, newest
// first. Powers SessionsPanel and the dashboard time-series chart.
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

// --- Users (admin only) ---------------------------------------------

export interface UserRow {
    id: string;
    username: string;
    role: "admin" | "operator" | "viewer";
}

export async function listUsers(): Promise<UserRow[]> {
    const j = await authJSON<{ users: UserRow[] }>("/api/v1/users");
    return j.users;
}

export async function createUser(
    username: string,
    password: string,
    role: UserRow["role"],
): Promise<UserRow> {
    return authJSON<UserRow>("/api/v1/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role }),
    });
}

export async function updateUser(
    id: string,
    patch: { role?: UserRow["role"]; password?: string },
): Promise<UserRow> {
    return authJSON<UserRow>(`/api/v1/users/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
    });
}

export async function deleteUser(id: string): Promise<void> {
    await authFetch(`/api/v1/users/${id}`, { method: "DELETE" });
}

// --- Admin settings (admin only) -----------------------------------
//
// The server exposes a small set of runtime-tunable policy knobs
// (token TTLs, release channel, presigned TTL, mesh discovery) via
// GET/PUT/DELETE on /api/v1/admin/settings/:key. Values cross the
// wire as typed JSON — number for durations/ints, bool, string.

export type SettingType =
    | "duration_seconds"
    | "bool"
    | "string"
    | "int"
    | "string_list";

// SettingDescriptor mirrors internal/settings.SettingDescriptor. The
// effective value is what the server is currently using; db/yaml hold
// the raw override / fallback values for the UI to show "source"
// hints next to each row.
export interface SettingDescriptor {
    key: string;
    type: SettingType;
    section: string;
    label: string;
    description: string;
    default: unknown;
    yaml?: unknown;
    db?: unknown;
    effective: unknown;
    source: "db" | "yaml" | "default";
}

export async function listSettings(): Promise<SettingDescriptor[]> {
    const j = await authJSON<{ settings: SettingDescriptor[] }>(
        "/api/v1/admin/settings",
    );
    return j.settings;
}

// updateSetting sends the *typed* value (the caller has already
// converted a form string to a number / bool). The server re-validates
// against the registered type.
export async function updateSetting(
    key: string,
    value: unknown,
): Promise<void> {
    await authFetch(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value }),
    });
}

export async function resetSetting(key: string): Promise<void> {
    await authFetch(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
        method: "DELETE",
    });
}

// --- Server info ----------------------------------------------------
// A thin roll-up of build metadata + live counts. Backed by
// GET /api/v1/info on the server; intended for low-frequency polling
// from the status bar.

export interface ServerInfo {
    // Build identity. git_repo is the canonical "<owner>/<name>"
    // path on GitHub — the status bar formats it into clickable
    // /releases/tag/v<version> links.
    version: string;
    commit: string;
    date: string;
    git_repo?: string;

    // Process lifetime. started_at is RFC3339 (kept for backwards
    // compat); started_at_unix is what the status-bar uptime ticker
    // does math against because Date.parse() drift is annoying.
    started_at: string;
    started_at_unix?: number;

    // Runtime stats — sampled per request.
    goroutines?: number;
    mem_alloc_bytes?: number;

    // Network identity.
    public_addr: string;

    // Counts. session_count is the legacy "live agent registry"
    // value; new code reads live_session_count / total_session_count
    // (DB ground truth) and host_count / live_host_count.
    session_count: number;
    live_session_count?: number;
    total_session_count?: number;
    host_count?: number;
    live_host_count?: number;
}

export async function getServerInfo(): Promise<ServerInfo> {
    return authJSON<ServerInfo>("/api/v1/info");
}

// --- PAT tokens -----------------------------------------------------
//
// Admin-only surface for provisioning enrollment tokens. The historical
// "PAT" name survives in tables, code paths, and URL segments because
// renaming the wire surface would be a bigger change; the user-facing
// UI calls them enrollment tokens (see EnrollmentPage). They are NOT
// account-scoped API tokens — they're one-shot agent-bootstrap secrets
// that burn on first /enroll and are replaced by an mTLS cert.
//
// The plaintext token (`plt_*`) only ever comes back in the response
// to POST; every subsequent list / get strips it and returns just
// metadata.

export interface EnrollmentTokenListItem {
    token_id: string;
    description?: string;
    issued_by_user: string;
    issued_at: string;
    expires_at: string;
    max_uses: number;
    uses: number;
    binding_machine_id?: string;
    binding_host_alias?: string;
    revoked: boolean;
    revoked_at?: string;
    revoked_reason?: string;
    status: "pending" | "consumed" | "expired" | "revoked";
}

export interface IssueEnrollmentTokenResponse {
    token_id: string;
    token: string; // plt_<id>.<secret> — only time plaintext is exposed
    expires_at: string;
    issued_at: string;
    max_uses: number;
    description?: string;
}

export interface IssueEnrollmentTokenRequest {
    description?: string;
    ttl_seconds?: number;
    max_uses?: number;
    binding_machine_id?: string;
    binding_host_alias?: string;
}

export async function listEnrollmentTokens(pid: string, includeInactive = false): Promise<EnrollmentTokenListItem[]> {
    const q = includeInactive ? "?include_inactive=true" : "";
    const j = await authJSON<{ tokens: EnrollmentTokenListItem[] }>(`/api/v1/projects/${pid}/pat-tokens${q}`);
    return j.tokens ?? [];
}

export async function issueEnrollmentToken(pid: string, req: IssueEnrollmentTokenRequest): Promise<IssueEnrollmentTokenResponse> {
    return authJSON<IssueEnrollmentTokenResponse>(`/api/v1/projects/${pid}/pat-tokens`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
    });
}

export async function revokeEnrollmentToken(pid: string, tokenID: string, reason?: string): Promise<void> {
    const q = reason ? `?reason=${encodeURIComponent(reason)}` : "";
    const r = await authFetch(`/api/v1/projects/${pid}/pat-tokens/${tokenID}${q}`, { method: "DELETE" });
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}

// --- Install artifacts ----------------------------------------------
//
// "Generate a one-shot curl command to install an agent". The returned
// install_command is ready to paste.

export interface InstallArtifactListItem {
    download_id: string;
    project_id: string;
    issued_by_user: string;
    issued_at: string;
    expires_at: string;
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    pat_ttl_seconds: number;
    pat_binding_machine_id?: string;
    pat_description?: string;
    consumed_at?: string;
    consumed_ip?: string;
    consumed_pat_id?: string;
    revoked: boolean;
    revoked_at?: string;
    status: "pending" | "consumed" | "expired" | "revoked";
}

export interface IssueInstallResponse {
    download_id: string;
    download_token: string; // dl_<id>.<secret>
    expires_at: string;
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    install_command: string; // "curl -fsSL ... | sh"
}

export interface IssueInstallRequest {
    server_endpoint: string;
    target_os?: string;
    target_arch?: string;
    ttl_seconds?: number;
    pat_ttl_seconds?: number;
    pat_binding_machine_id?: string;
    pat_description?: string;
}

export async function listInstallArtifacts(pid: string, includeInactive = false): Promise<InstallArtifactListItem[]> {
    const q = includeInactive ? "?include_inactive=true" : "";
    const j = await authJSON<{ install_artifacts: InstallArtifactListItem[] }>(
        `/api/v1/projects/${pid}/install-artifacts${q}`,
    );
    return j.install_artifacts ?? [];
}

export async function issueInstallArtifact(pid: string, req: IssueInstallRequest): Promise<IssueInstallResponse> {
    return authJSON<IssueInstallResponse>(`/api/v1/projects/${pid}/install-artifacts`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
    });
}

export async function revokeInstallArtifact(pid: string, downloadID: string, reason?: string): Promise<void> {
    const q = reason ? `?reason=${encodeURIComponent(reason)}` : "";
    const r = await authFetch(`/api/v1/projects/${pid}/install-artifacts/${downloadID}${q}`, { method: "DELETE" });
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}

// InstallPlatform is one (os, arch) pair the active channel's manifest
// pins. The Issue Install dialog uses the list to populate its picker
// so admins can only choose targets the distributor can actually serve.
export interface InstallPlatform {
    os: string;
    arch: string;
}

export interface InstallPlatformsResponse {
    channel: string;
    // Empty when no manifest has been published yet — the response is
    // still 200 so the dialog can render a clear "publish first" hint.
    version: string;
    platforms: InstallPlatform[];
}

export async function listInstallPlatforms(): Promise<InstallPlatformsResponse> {
    return authJSON<InstallPlatformsResponse>("/api/v1/install/platforms");
}

// --- Activities / audit timeline ------------------------------------
//
// A single append-only `activities` table on the server backs both the
// live "what just happened" view and the compliance export. The legacy
// audit-export endpoint has been retired; use these entry points.

export type ActivityOutcome = "success" | "denied" | "error";

export interface ActivityItem {
    id: number;
    at: string;
    project_id: string | null;
    actor_type: string;
    actor_user: string;
    actor_ip?: string;
    actor_ua?: string;
    actor_token_id?: string;
    category: string;
    action: string;
    target_type?: string;
    target_id?: string;
    target_label?: string;
    outcome: ActivityOutcome;
    error?: string;
    duration_ms?: number;
    request_id?: string;
    session_id?: string;
    meta?: Record<string, unknown> | string;
}

// ActivitySource is the high-level "where did this row come from"
// bucket the activities page exposes as a segment control. Each
// value maps to one or more raw actor_type values on the server (see
// expandSourceAlias in handler_activities_v1.go); keeping the alias
// here means dashboards and the URL can stay stable when we add a
// new actor type.
export type ActivitySource = "human" | "agent" | "system";

export interface ListActivitiesOpts {
    from?: Date;
    to?: Date;
    category?: string[];
    action?: string[];
    actor?: string;
    actorType?: string[];
    sources?: ActivitySource[];
    outcome?: ActivityOutcome;
    sessionId?: string;
    targetType?: string;
    targetId?: string;
    q?: string;
    limit?: number;
    cursor?: string;
    includeGlobal?: boolean;
    includeTotal?: boolean;
}

export interface ListActivitiesResponse {
    items: ActivityItem[];
    next_cursor?: string;
    total?: number;
}

// buildActivityParams centralises the URLSearchParams construction so
// list + export hit the same query shape the backend expects.
function buildActivityParams(opts: ListActivitiesOpts): URLSearchParams {
    const p = new URLSearchParams();
    if (opts.from) p.set("from", opts.from.toISOString());
    if (opts.to) p.set("to", opts.to.toISOString());
    if (opts.category && opts.category.length) p.set("category", opts.category.join(","));
    if (opts.action) {
        for (const a of opts.action) p.append("action", a);
    }
    if (opts.actor) p.set("actor", opts.actor);
    if (opts.actorType && opts.actorType.length) p.set("actor_type", opts.actorType.join(","));
    if (opts.sources && opts.sources.length) p.set("source", opts.sources.join(","));
    if (opts.outcome) p.set("outcome", opts.outcome);
    if (opts.sessionId) p.set("session_id", opts.sessionId);
    if (opts.targetType) p.set("target_type", opts.targetType);
    if (opts.targetId) p.set("target_id", opts.targetId);
    if (opts.q) p.set("q", opts.q);
    if (opts.limit) p.set("limit", String(opts.limit));
    if (opts.cursor) p.set("cursor", opts.cursor);
    if (opts.includeGlobal) p.set("include_global", "true");
    if (opts.includeTotal) p.set("include_total", "true");
    return p;
}

export async function listProjectActivities(
    pid: string,
    opts: ListActivitiesOpts = {},
): Promise<ListActivitiesResponse> {
    const qs = buildActivityParams(opts).toString();
    const path = `/api/v1/projects/${pid}/activities${qs ? "?" + qs : ""}`;
    return authJSON<ListActivitiesResponse>(path);
}

export async function listGlobalActivities(
    opts: ListActivitiesOpts = {},
): Promise<ListActivitiesResponse> {
    const qs = buildActivityParams(opts).toString();
    const path = `/api/v1/activities${qs ? "?" + qs : ""}`;
    return authJSON<ListActivitiesResponse>(path);
}

export async function exportProjectActivitiesBlob(
    pid: string,
    opts: ListActivitiesOpts & { format?: "jsonl" | "csv" } = {},
): Promise<Blob> {
    const p = buildActivityParams(opts);
    p.set("format", opts.format ?? "jsonl");
    const r = await authFetch(`/api/v1/projects/${pid}/activities/export?${p.toString()}`);
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
    return r.blob();
}

// --- Topology -------------------------------------------------------
//
// Mirrors core.TopologySnapshot on the server. `machines` is the
// compound-parent layer (host + sysinfo + sessions); `mesh_nodes`
// cross-references mesh NodeIDs to their machine; `links` is the
// undirected edge set with traffic counters.

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
