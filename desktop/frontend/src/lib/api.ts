// Typed wrappers over the /api/v1 surface introduced by the redesign
// (Projects, Hosts, Listeners, Sessions, Dispatch). Thin on purpose —
// each function is one authJSON call. Response shapes mirror the Go
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
}

export interface Listener {
    id: string;
    project_id: string;
    host: string;
    port: number;
    public_ip?: string;
    shell_path?: string;
    created_at: string;
}

export interface SessionRow {
    id: string;
    project_id: string;
    listener_id: string;
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

export interface DispatchResult {
    session_hash: string;
    host_id: string;
    output: string;
    error?: string;
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

export async function listHostSessions(pid: string, hid: string): Promise<SessionRow[]> {
    const j = await authJSON<{ sessions: SessionRow[] }>(
        `/api/v1/projects/${pid}/hosts/${hid}/sessions`,
    );
    return j.sessions;
}

// --- Listeners -------------------------------------------------------

export async function listListeners(pid: string): Promise<Listener[]> {
    const j = await authJSON<{ listeners: Listener[] }>(`/api/v1/projects/${pid}/listeners`);
    return j.listeners;
}

export async function createListener(pid: string, host: string, port: number): Promise<Listener> {
    return authJSON<Listener>(`/api/v1/projects/${pid}/listeners`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host, port }),
    });
}

export async function deleteListener(pid: string, lid: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/listeners/${lid}`, { method: "DELETE" });
}

// --- Dispatch --------------------------------------------------------

export async function dispatchCommand(
    pid: string,
    command: string,
    timeout = 3,
): Promise<DispatchResult[]> {
    const j = await authJSON<{ count: number; results: DispatchResult[] }>(
        `/api/v1/projects/${pid}/dispatch`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ command, timeout }),
        },
    );
    return j.results;
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
