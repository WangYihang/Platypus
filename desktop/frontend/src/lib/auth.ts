// Multi-session auth pool. One Session per saved ServerProfile lives
// in memory; the long-lived opaque session_token is cached in
// localStorage under a single namespaced key so the rail can keep
// multiple workspaces warm at once.
//
// Phase 2 cutover: there is no access/refresh pair anymore. A single
// pst_<id>.<secret> session token authenticates every request; the
// server enforces a 24h sliding idle window plus a 30d hard cap and
// invalidates the row on logout / password-change. Without refresh
// rotation, this module is a lot simpler than the old one.

import {
    ServerProfile,
    addServer,
    getActiveServerId,
    getServer,
    listServers,
    normaliseURL,
    onServersChange,
    removeServer,
    setActiveServerId,
} from "./servers";

export interface SessionUser {
    id: string;
    username: string;
    role: "admin" | "operator" | "viewer";
}

export interface Session {
    serverId: string;
    serverURL: string;
    sessionToken: string;
    tokenId: string;
    user: SessionUser;
    // Server-supplied deadlines so UI can show "expires in N days"
    // without polling. expiresAt is the hard cap; idleExpiresAt slides
    // forward on every authenticated request and is the practical
    // bound on inactivity.
    expiresAt: number;
    idleExpiresAt: number;
}

// Persisted shape — only the long-lived session token plus enough
// metadata to render a warm "logged in" state on boot. We avoid
// caching the full Session here so a stale role (e.g. demoted user)
// doesn't survive a page reload — the next authFetch hydrates from
// the server's view.
interface PersistedEntry {
    sessionToken: string;
    tokenId?: string;
    user?: SessionUser;
}
type PersistedSessions = Record<string, PersistedEntry>;

const LS_SESSIONS = "platypus.sessions";

const sessions = new Map<string, Session>();
const listeners = new Set<() => void>();
const activeListeners = new Set<() => void>();
let lastActiveId: string | null = null;

function emit() {
    for (const fn of listeners) {
        try {
            fn();
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("session listener threw:", err);
        }
    }
}

function emitActive() {
    for (const fn of activeListeners) {
        try {
            fn();
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("active listener threw:", err);
        }
    }
}

// Watch servers.ts so `setActiveServerId` → our `onActiveChange`.
onServersChange(() => {
    const current = getActiveServerId();
    if (current !== lastActiveId) {
        lastActiveId = current;
        emitActive();
    }
});

export function onSessionChange(fn: () => void): () => void {
    listeners.add(fn);
    return () => {
        listeners.delete(fn);
    };
}

export function onActiveChange(fn: () => void): () => void {
    activeListeners.add(fn);
    return () => {
        activeListeners.delete(fn);
    };
}

// --- Persistence helpers --------------------------------------------

function readPersisted(): PersistedSessions {
    try {
        const raw = localStorage.getItem(LS_SESSIONS);
        if (!raw) return {};
        const parsed = JSON.parse(raw) as unknown;
        if (!parsed || typeof parsed !== "object") return {};
        const out: PersistedSessions = {};
        for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
            if (!v || typeof v !== "object") continue;
            const entry = v as Partial<PersistedEntry>;
            if (typeof entry.sessionToken !== "string") continue;
            out[k] = {
                sessionToken: entry.sessionToken,
                tokenId: entry.tokenId,
                user: entry.user,
            };
        }
        return out;
    } catch {
        return {};
    }
}

function writePersisted(map: PersistedSessions): void {
    try {
        if (Object.keys(map).length === 0) {
            localStorage.removeItem(LS_SESSIONS);
        } else {
            localStorage.setItem(LS_SESSIONS, JSON.stringify(map));
        }
    } catch {
        // ignore storage errors
    }
}

function persistSession(s: Session): void {
    const map = readPersisted();
    map[s.serverId] = {
        sessionToken: s.sessionToken,
        tokenId: s.tokenId,
        user: s.user,
    };
    writePersisted(map);
}

function dropPersisted(serverId: string): void {
    const map = readPersisted();
    if (!(serverId in map)) return;
    delete map[serverId];
    writePersisted(map);
}

// --- HTTP primitives ------------------------------------------------

async function postJSON<T>(url: string, body: unknown): Promise<T> {
    const r = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
    return (await r.json()) as T;
}

interface LoginResponse {
    session_token: string;
    token_id: string;
    expires_at: string;
    idle_expires_at: string;
    user?: SessionUser;
}

function storeSession(
    profile: ServerProfile,
    resp: LoginResponse,
    fallbackUser?: SessionUser,
): Session {
    const user = resp.user ?? fallbackUser;
    if (!user) throw new Error("auth response missing user");
    const session: Session = {
        serverId: profile.id,
        serverURL: normaliseURL(profile.url),
        sessionToken: resp.session_token,
        tokenId: resp.token_id,
        user,
        expiresAt: Date.parse(resp.expires_at),
        idleExpiresAt: Date.parse(resp.idle_expires_at),
    };
    sessions.set(profile.id, session);
    persistSession(session);
    emit();
    return session;
}

// hydrateFromPersisted rebuilds an in-memory Session from a persisted
// entry without contacting the server. The token's per-row expiries
// aren't persisted so we leave them at 0; the very next authFetch
// will either succeed (user still logged in) or 401 (session
// revoked / idle-expired) and the caller routes to /login.
function hydrateFromPersisted(profile: ServerProfile, entry: PersistedEntry): Session | null {
    if (!entry.user) return null;
    const session: Session = {
        serverId: profile.id,
        serverURL: normaliseURL(profile.url),
        sessionToken: entry.sessionToken,
        tokenId: entry.tokenId ?? "",
        user: entry.user,
        expiresAt: 0,
        idleExpiresAt: 0,
    };
    sessions.set(profile.id, session);
    return session;
}

// --- Public reads ---------------------------------------------------

export function getSession(serverId?: string): Session | null {
    const id = serverId ?? getActiveServerId();
    if (!id) return null;
    if (sessions.has(id)) {
        return sessions.get(id) ?? null;
    }
    // Cold-boot path: rebuild from localStorage if we have a token.
    const profile = getServer(id);
    const persisted = readPersisted()[id];
    if (!profile || !persisted) return null;
    return hydrateFromPersisted(profile, persisted);
}

export function getSessionUser(): SessionUser | null {
    return getSession()?.user ?? null;
}

export function hasPersistedSession(serverId: string): boolean {
    return serverId in readPersisted();
}

export function listLiveSessionIds(): string[] {
    return Array.from(sessions.keys());
}

// --- Login / bootstrap ---------------------------------------------

function findOrCreateProfile(url: string): ServerProfile {
    const norm = normaliseURL(url);
    const existing = listServers().find((s) => s.url === norm);
    if (existing) return existing;
    return addServer({ url: norm });
}

export interface LoginOpts {
    profile?: ServerProfile;
}

export async function login(
    urlOrProfile: string | ServerProfile,
    username: string,
    password: string,
): Promise<ServerProfile> {
    const profile =
        typeof urlOrProfile === "string"
            ? findOrCreateProfile(urlOrProfile)
            : urlOrProfile;
    const resp = await postJSON<LoginResponse>(
        profile.url + "/api/v1/auth/login",
        { username, password },
    );
    storeSession(profile, resp);
    setActiveServerId(profile.id);
    return profile;
}

export async function bootstrap(
    urlOrProfile: string | ServerProfile,
    secret: string,
    username: string,
    password: string,
): Promise<ServerProfile> {
    const profile =
        typeof urlOrProfile === "string"
            ? findOrCreateProfile(urlOrProfile)
            : urlOrProfile;
    const resp = await postJSON<LoginResponse>(
        profile.url + "/api/v1/auth/bootstrap",
        { secret, username, password },
    );
    storeSession(profile, resp);
    setActiveServerId(profile.id);
    return profile;
}

// switchServer flips the active pointer and reports whether the
// target has a live session. Cold-boots one from localStorage if
// possible; otherwise the caller routes to /login.
export async function switchServer(id: string): Promise<{ loggedIn: boolean }> {
    setActiveServerId(id);
    if (sessions.has(id)) {
        return { loggedIn: true };
    }
    const profile = getServer(id);
    const persisted = readPersisted()[id];
    if (!profile || !persisted) return { loggedIn: false };
    const session = hydrateFromPersisted(profile, persisted);
    return { loggedIn: session !== null };
}

// forgetServer drops the in-memory session and the persisted token
// for one server. The ServerProfile itself stays — "Sign out" in
// Manage Servers uses this; "Remove server" also calls through to
// servers.removeServer() afterwards.
export function forgetServer(id: string): void {
    sessions.delete(id);
    dropPersisted(id);
    emit();
}

export function forgetAndRemoveServer(id: string): void {
    removeServer(id);
    forgetServer(id);
}

// --- Change password / logout --------------------------------------

export async function changePassword(
    oldPassword: string,
    newPassword: string,
): Promise<void> {
    // Server preserves the caller's session through the cascade, so
    // we don't drop local state on success — only on a failure that
    // bounced us out anyway.
    await authFetch("/api/v1/auth/password", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
    // No local rotation needed — the caller's pst_ token is still
    // valid (the cascade kills only "other" sessions). Re-emit so
    // any UI subscribed to session changes refreshes.
    emit();
}

export async function logout(): Promise<void> {
    const s = getSession();
    if (!s) return;
    const url = s.serverURL + "/api/v1/auth/logout";
    const token = s.sessionToken;
    sessions.delete(s.serverId);
    dropPersisted(s.serverId);
    emit();
    try {
        // /logout sits behind RequireAuth — bearer carries the
        // session id. No body needed.
        await fetch(url, {
            method: "POST",
            headers: { Authorization: "Bearer " + token },
        });
    } catch {
        // ignore — local state is already cleared
    }
}

// --- authFetch ------------------------------------------------------

export class StaleServerResponseError extends Error {
    constructor(public serverIdAtCall: string, public activeServerIdNow: string | null) {
        super(`response for server ${serverIdAtCall} arrived after switch to ${activeServerIdNow ?? "none"}`);
    }
}

export class SessionExpiredError extends Error {
    constructor(public serverId: string) {
        super(`session expired for server ${serverId}`);
        this.name = "SessionExpiredError";
    }
}

export async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
    const id = getActiveServerId();
    if (!id) throw new Error("not logged in");
    let session = sessions.get(id);
    if (!session) {
        // Cold path: try the persisted token before failing.
        const profile = getServer(id);
        const persisted = readPersisted()[id];
        if (!profile || !persisted) throw new Error("not logged in");
        const hydrated = hydrateFromPersisted(profile, persisted);
        if (!hydrated) throw new Error("not logged in");
        session = hydrated;
    }

    const headers = new Headers(init.headers);
    headers.set("Authorization", "Bearer " + session.sessionToken);
    const r = await fetch(session.serverURL + path, { ...init, headers });

    if (r.status === 401) {
        // Session revoked / idle-expired — there is no refresh dance
        // anymore. Drop local state so the caller can route to
        // /login cleanly.
        forgetServer(id);
        throw new SessionExpiredError(id);
    }

    // Cross-server race guard: if the user switched while the fetch
    // was in flight, reject so the caller doesn't merge a stale
    // response into the new server's state.
    const active = getActiveServerId();
    if (active !== id) {
        throw new StaleServerResponseError(id, active);
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

// --- Public (unauthenticated) probe --------------------------------

export interface PublicServerInfo {
    product: string;
    admin_bootstrapped: boolean;
}

export class ProbeError extends Error {
    constructor(
        message: string,
        public readonly cause?: unknown,
    ) {
        super(message);
        this.name = "ProbeError";
    }
}

function friendlyFetchError(err: unknown): string {
    const raw = err instanceof Error ? err.message : String(err);
    if (/Failed to fetch|NetworkError|ECONNREFUSED|ENOTFOUND/i.test(raw)) {
        return "Couldn't reach this server. Check the URL and that the server is running.";
    }
    return raw;
}

export async function probeServer(url: string): Promise<PublicServerInfo> {
    const base = normaliseURL(url);
    let r: Response;
    try {
        r = await fetch(base + "/api/v1/auth/info");
    } catch (err) {
        throw new ProbeError(friendlyFetchError(err), err);
    }
    if (r.status === 404) {
        throw new ProbeError(
            "This URL responded but doesn't look like a Platypus server. Double-check the address.",
        );
    }
    if (!r.ok) {
        throw new ProbeError(
            `Server responded with ${r.status}. Check the URL or try again in a moment.`,
        );
    }
    try {
        const body = (await r.json()) as PublicServerInfo;
        if (body.product !== "platypus") {
            throw new ProbeError(
                "This URL responded but doesn't look like a Platypus server. Double-check the address.",
            );
        }
        return body;
    } catch (err) {
        if (err instanceof ProbeError) throw err;
        throw new ProbeError(
            "The server returned an unexpected response. It may not be a Platypus server.",
            err,
        );
    }
}

// --- Compat shim ---------------------------------------------------

// refresh() existed in the old JWT pair model. The session model has
// no refresh — the export is kept as a deprecation no-op so any
// imports still in flight don't fail at module load. New code should
// not call this.
export async function refresh(_serverId?: string): Promise<boolean> {
    return false;
}
