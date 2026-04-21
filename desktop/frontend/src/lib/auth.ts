// JWT session store + authenticated fetch. Lives next to lib/format and
// lib/time so pages have one place to look for non-UI utilities.
//
// Storage model:
// - access token: memory only (short-lived, rotated silently by the
//   refresh flow). Survives full-page reloads only via refresh().
// - refresh token: localStorage, so a browser refresh or a desktop
//   app relaunch doesn't force the user through /login again.
// - user: memory, filled from the login response and refreshed on
//   boot via /auth/refresh.

export interface SessionUser {
    id: string;
    username: string;
    role: "admin" | "operator" | "viewer";
}

interface Session {
    serverURL: string;
    accessToken: string;
    refreshToken: string;
    user: SessionUser;
    accessIssuedAt: number; // ms since epoch, for basic pre-emptive refresh
}

const LS_SERVER_URL = "platypus.server_url";
const LS_REFRESH = "platypus.refresh_token";

let session: Session | null = null;
const listeners = new Set<() => void>();

function notify() {
    for (const l of listeners) l();
}

export function onSessionChange(fn: () => void): () => void {
    listeners.add(fn);
    return () => listeners.delete(fn);
}

export function getSession(): Session | null {
    return session;
}

export function getSessionUser(): SessionUser | null {
    return session?.user ?? null;
}

function persistRefresh() {
    if (session) {
        localStorage.setItem(LS_SERVER_URL, session.serverURL);
        localStorage.setItem(LS_REFRESH, session.refreshToken);
    } else {
        localStorage.removeItem(LS_SERVER_URL);
        localStorage.removeItem(LS_REFRESH);
    }
}

function normaliseURL(url: string): string {
    return url.replace(/\/+$/, "");
}

// --- HTTP primitives --------------------------------------------------

async function postJSON<T>(url: string, body: unknown): Promise<T> {
    const r = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
    return (await r.json()) as T;
}

interface TokenPairResponse {
    access_token: string;
    refresh_token: string;
    user?: SessionUser;
}

function storeTokens(serverURL: string, pair: TokenPairResponse, fallbackUser?: SessionUser) {
    const user = pair.user ?? fallbackUser;
    if (!user) throw new Error("auth response missing user");
    session = {
        serverURL: normaliseURL(serverURL),
        accessToken: pair.access_token,
        refreshToken: pair.refresh_token,
        user,
        accessIssuedAt: Date.now(),
    };
    persistRefresh();
    notify();
}

// --- Public API --------------------------------------------------------

export async function login(serverURL: string, username: string, password: string): Promise<void> {
    const base = normaliseURL(serverURL);
    const pair = await postJSON<TokenPairResponse>(base + "/api/v1/auth/login", {
        username,
        password,
    });
    storeTokens(base, pair);
}

export async function bootstrap(
    serverURL: string,
    secret: string,
    username: string,
    password: string,
): Promise<void> {
    const base = normaliseURL(serverURL);
    const pair = await postJSON<TokenPairResponse>(base + "/api/v1/auth/bootstrap", {
        secret,
        username,
        password,
    });
    storeTokens(base, pair);
}

// refresh swaps the stored refresh token for a fresh access+refresh pair.
// Called on boot (to rehydrate the session from localStorage) and on 401
// from an authenticated fetch. Returns false if the server rejected the
// stored refresh token — caller should send the user back to login.
export async function refresh(): Promise<boolean> {
    const serverURL = localStorage.getItem(LS_SERVER_URL);
    const refreshToken = localStorage.getItem(LS_REFRESH);
    if (!serverURL || !refreshToken) return false;
    try {
        const pair = await postJSON<TokenPairResponse>(serverURL + "/api/v1/auth/refresh", {
            refresh_token: refreshToken,
        });
        // The refresh endpoint doesn't always return the user field;
        // keep the one we had from login, or fetch fresh.
        const previousUser = session?.user;
        storeTokens(serverURL, pair, previousUser);
        return true;
    } catch {
        session = null;
        persistRefresh();
        notify();
        return false;
    }
}

export async function logout(): Promise<void> {
    if (!session) return;
    const url = session.serverURL + "/api/v1/auth/logout";
    const token = session.refreshToken;
    session = null;
    persistRefresh();
    notify();
    // Best-effort — if the server is unreachable the client session is
    // already cleared, which is the important part.
    try {
        await postJSON(url, { refresh_token: token });
    } catch {
        // ignore
    }
}

// authFetch issues an authenticated request against the current session's
// server. Transparently refreshes on 401 once before giving up.
export async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
    if (!session) throw new Error("not logged in");
    const doFetch = async () => {
        if (!session) throw new Error("not logged in");
        const headers = new Headers(init.headers);
        headers.set("Authorization", "Bearer " + session.accessToken);
        return fetch(session.serverURL + path, { ...init, headers });
    };

    let r = await doFetch();
    if (r.status === 401) {
        const ok = await refresh();
        if (!ok) throw new Error("session expired");
        r = await doFetch();
    }
    if (!r.ok) {
        const body = await r.text();
        throw new Error(`${r.status}: ${body}`);
    }
    return r;
}

export async function authJSON<T>(path: string, init?: RequestInit): Promise<T> {
    const r = await authFetch(path, init);
    return r.json();
}
