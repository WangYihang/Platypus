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

export interface ListenerResp {
    id: string;
    project_id: string;
    host: string;
    port: number;
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

export async function createListener(
    backendURL: string,
    token: string,
    projectID: string,
    host: string,
    port: number,
): Promise<ListenerResp> {
    return postJSON<ListenerResp>(
        `${backendURL}/api/v1/projects/${projectID}/listeners`,
        { host, port },
        token,
    );
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
