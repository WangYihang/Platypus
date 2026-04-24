// Tiny REST helpers used by globalSetup to seed the database. Avoid
// pulling in axios/got — these run before tests so a fetch wrapper
// keeps the dep surface small.

export interface BootstrapBody {
    secret: string;
    username: string;
    password: string;
}

export interface LoginBody {
    username: string;
    password: string;
}

export interface TokenPair {
    access_token: string;
    refresh_token: string;
    user: { id: string; username: string; role: "admin" | "operator" | "viewer" };
}

export interface ProjectResp {
    id: string;
    slug: string;
    name: string;
}

async function postJSON<T>(url: string, body: unknown, token?: string): Promise<T> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const r = await fetch(url, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
    });
    if (!r.ok) {
        throw new Error(
            `${url} → ${r.status}: ${await r.text()}`,
        );
    }
    return (await r.json()) as T;
}

export async function bootstrapAdmin(
    backendURL: string,
    body: BootstrapBody,
): Promise<TokenPair> {
    return postJSON<TokenPair>(`${backendURL}/api/v1/auth/bootstrap`, body);
}

export async function loginAPI(backendURL: string, body: LoginBody): Promise<TokenPair> {
    return postJSON<TokenPair>(`${backendURL}/api/v1/auth/login`, body);
}

export async function createProject(
    backendURL: string,
    token: string,
    name: string,
    slug: string,
): Promise<ProjectResp> {
    return postJSON<ProjectResp>(
        `${backendURL}/api/v1/projects`,
        { name, slug },
        token,
    );
}

export interface PATResponse {
    token_id: string;
    token: string; // plaintext "plt_<id>.<secret>" — only ever returned here
    expires_at: string;
    issued_at: string;
    max_uses: number;
    description?: string;
}

// issuePAT mints a fresh PAT for the given project. The plaintext
// token lives only in this response; callers should stash it.
export async function issuePAT(
    backendURL: string,
    adminToken: string,
    projectID: string,
    opts: { description?: string; ttl_seconds?: number; max_uses?: number } = {},
): Promise<PATResponse> {
    return postJSON<PATResponse>(
        `${backendURL}/api/v1/projects/${projectID}/pat-tokens`,
        {
            description: opts.description ?? "e2e",
            ttl_seconds: opts.ttl_seconds ?? 3600,
            max_uses: opts.max_uses ?? 0,
        },
        adminToken,
    );
}

export interface ProjectCAResp {
    project_id: string;
    cert_pem: string;
    created_at: string;
    created_by_user: string;
    serial_counter: number;
}

// getProjectCA fetches the PEM-encoded CA cert for the project. The
// agent loads this (base64-wrapped) from PLATYPUS_PROJECT_CA on
// startup to pin the server chain.
export async function getProjectCA(
    backendURL: string,
    adminToken: string,
    projectID: string,
): Promise<ProjectCAResp> {
    const r = await fetch(`${backendURL}/api/v1/projects/${projectID}/ca`, {
        headers: { Authorization: `Bearer ${adminToken}` },
    });
    if (!r.ok) throw new Error(`get project CA → ${r.status}: ${await r.text()}`);
    return (await r.json()) as ProjectCAResp;
}

export async function listProjects(
    backendURL: string,
    token: string,
): Promise<ProjectResp[]> {
    const r = await fetch(`${backendURL}/api/v1/projects`, {
        headers: { Authorization: `Bearer ${token}` },
    });
    if (!r.ok) throw new Error(`list projects → ${r.status}: ${await r.text()}`);
    const j = (await r.json()) as { projects?: ProjectResp[] } | ProjectResp[];
    return Array.isArray(j) ? j : j.projects || [];
}

export interface SessionResp {
    id: string;
    project_id: string;
    listener_id: string;
    host_id: string;
    user?: string;
    remote_addr?: string;
    group_dispatch: boolean;
    connected_at: string;
    disconnected_at?: string;
}

export async function listProjectSessions(
    backendURL: string,
    token: string,
    projectID: string,
    opts: { live?: boolean } = {},
): Promise<SessionResp[]> {
    const q = new URLSearchParams();
    if (opts.live !== undefined) q.set("live", String(opts.live));
    const url = `${backendURL}/api/v1/projects/${projectID}/sessions${q.toString() ? `?${q}` : ""}`;
    const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } });
    if (!r.ok) throw new Error(`list project sessions → ${r.status}: ${await r.text()}`);
    const j = (await r.json()) as { sessions?: SessionResp[] };
    return j.sessions || [];
}

// waitForSessions polls the per-project sessions endpoint. In the
// v2 server only host rows are persisted on agent connect — logical
// sessions aren't created automatically — so new specs should prefer
// waitForHosts. Kept for any legacy caller that asserts directly on
// sessions.
export async function waitForSessions(
    backendURL: string,
    token: string,
    projectID: string,
    min: number,
    timeoutMs = 15_000,
): Promise<SessionResp[]> {
    const deadline = Date.now() + timeoutMs;
    let last: SessionResp[] = [];
    while (Date.now() < deadline) {
        try {
            last = await listProjectSessions(backendURL, token, projectID, { live: true });
            if (last.length >= min) return last;
        } catch {
            /* swallow until deadline */
        }
        await new Promise((r) => setTimeout(r, 250));
    }
    throw new Error(
        `expected ≥${min} live session(s) in project ${projectID} within ${timeoutMs}ms (saw ${last.length})`,
    );
}

export interface HostResp {
    id: string;
    project_id: string;
    hostname?: string;
    online: boolean;
}

export async function listProjectHosts(
    backendURL: string,
    token: string,
    projectID: string,
): Promise<HostResp[]> {
    const r = await fetch(`${backendURL}/api/v1/projects/${projectID}/hosts`, {
        headers: { Authorization: `Bearer ${token}` },
    });
    if (!r.ok) throw new Error(`list project hosts → ${r.status}: ${await r.text()}`);
    const j = (await r.json()) as { hosts?: HostResp[] };
    return j.hosts || [];
}

// waitForHosts polls the per-project hosts endpoint until at least
// `min` rows appear. The v2 enroll flow upserts a host row on every
// agent enrollment (and on subsequent link connects), so this is the
// reliable "an agent is online" signal for specs that need to wait
// for their fixture agent to show up in the API.
export async function waitForHosts(
    backendURL: string,
    token: string,
    projectID: string,
    min: number,
    timeoutMs = 15_000,
): Promise<HostResp[]> {
    const deadline = Date.now() + timeoutMs;
    let last: HostResp[] = [];
    while (Date.now() < deadline) {
        try {
            last = await listProjectHosts(backendURL, token, projectID);
            if (last.length >= min) return last;
        } catch {
            /* swallow until deadline */
        }
        await new Promise((r) => setTimeout(r, 250));
    }
    throw new Error(
        `expected ≥${min} host(s) in project ${projectID} within ${timeoutMs}ms (saw ${last.length})`,
    );
}

// waitForBackend polls the login endpoint until it returns 401 (rather
// than connection-refused). 401 is the success signal — the server is
// up and reachable, just declining our empty body.
export async function waitForBackend(backendURL: string, timeoutMs = 30_000): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    let lastErr: unknown;
    while (Date.now() < deadline) {
        try {
            const r = await fetch(`${backendURL}/api/v1/auth/login`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: "{}",
            });
            // 4xx is fine — the server answered. 5xx still means alive,
            // but we treat it as ready too rather than block.
            if (r.status >= 400) return;
        } catch (e) {
            lastErr = e;
        }
        await new Promise((res) => setTimeout(res, 200));
    }
    throw new Error(
        `backend did not become reachable at ${backendURL} within ${timeoutMs}ms: ${String(lastErr)}`,
    );
}
