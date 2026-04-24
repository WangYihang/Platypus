// Multi-session auth pool. One Session per saved ServerProfile lives
// in memory; refresh tokens are cached in localStorage under a single
// namespaced key so the rail can keep multiple workspaces warm at
// once. Access tokens stay in RAM (short-lived, rotated by refresh).
//
// Public surface is kept deliberately close to the old single-session
// shape — `getSession()`, `authFetch()`, `login()`, `bootstrap()`,
// `refresh()`, `logout()`, `changePassword()` all still exist and
// target the *active* server. New helpers (`switchServer`,
// `forgetServer`, `onActiveChange`) drive the rail.

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
    accessToken: string;
    refreshToken: string;
    user: SessionUser;
    accessIssuedAt: number;
}

// Persisted shape — only the minimum needed to rebuild a Session on
// boot. We re-fetch the user via /auth/refresh so stale role changes
// don't stick around.
interface PersistedEntry {
    refreshToken: string;
    user?: SessionUser;
}
type PersistedSessions = Record<string, PersistedEntry>;

const LS_SESSIONS = "platypus.sessions";
const LEGACY_LS_REFRESH = "platypus.refresh_token";

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
            if (typeof entry.refreshToken !== "string") continue;
            out[k] = { refreshToken: entry.refreshToken, user: entry.user };
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
    map[s.serverId] = { refreshToken: s.refreshToken, user: s.user };
    writePersisted(map);
}

function dropPersisted(serverId: string): void {
    const map = readPersisted();
    if (!(serverId in map)) return;
    delete map[serverId];
    writePersisted(map);
}

// Lift the old single-session localStorage keys into the first pooled
// entry. We intentionally run this on first reference, not at module
// load, so tests that clear storage between specs get a fresh slate.
let migrated = false;
function migrateLegacy(): void {
    if (migrated) return;
    migrated = true;
    try {
        const legacyRefresh = localStorage.getItem(LEGACY_LS_REFRESH);
        if (!legacyRefresh) return;
        const profile = listServers()[0];
        if (!profile) {
            // servers.ts has its own migration that synthesises the
            // first profile from the legacy server_url; once that
            // lands we'll pair the refresh token with it on the next
            // boot. Nothing to do here yet.
            return;
        }
        const map = readPersisted();
        if (!map[profile.id]) {
            map[profile.id] = { refreshToken: legacyRefresh };
            writePersisted(map);
        }
        localStorage.removeItem(LEGACY_LS_REFRESH);
    } catch {
        // ignore
    }
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

interface TokenPairResponse {
    access_token: string;
    refresh_token: string;
    user?: SessionUser;
}

function storeTokens(
    profile: ServerProfile,
    pair: TokenPairResponse,
    fallbackUser?: SessionUser,
): Session {
    const user = pair.user ?? fallbackUser;
    if (!user) throw new Error("auth response missing user");
    const session: Session = {
        serverId: profile.id,
        serverURL: normaliseURL(profile.url),
        accessToken: pair.access_token,
        refreshToken: pair.refresh_token,
        user,
        accessIssuedAt: Date.now(),
    };
    sessions.set(profile.id, session);
    persistSession(session);
    emit();
    return session;
}

// --- Public reads ---------------------------------------------------

export function getSession(serverId?: string): Session | null {
    migrateLegacy();
    const id = serverId ?? getActiveServerId();
    if (!id) return null;
    return sessions.get(id) ?? null;
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

// findOrCreateProfile gives the old URL-keyed callers a path into the
// pool: if a profile already points at this URL, reuse it; otherwise
// add a fresh one. Callers that care about identity (rail / wizard)
// should pass an explicit profile instead.
function findOrCreateProfile(url: string): ServerProfile {
    const norm = normaliseURL(url);
    const existing = listServers().find((s) => s.url === norm);
    if (existing) return existing;
    return addServer({ url: norm });
}

export interface LoginOpts {
    // When the caller already registered the profile (wizard, rail
    // add-server), pass its id so we don't duplicate on reuse.
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
    const pair = await postJSON<TokenPairResponse>(
        profile.url + "/api/v1/auth/login",
        { username, password },
    );
    storeTokens(profile, pair);
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
    const pair = await postJSON<TokenPairResponse>(
        profile.url + "/api/v1/auth/bootstrap",
        { secret, username, password },
    );
    storeTokens(profile, pair);
    setActiveServerId(profile.id);
    return profile;
}

// refresh tries to swap the cached refresh token for a fresh pair.
// Returns true when the supplied server (or the active one, if none
// given) is now logged in.
export async function refresh(serverId?: string): Promise<boolean> {
    migrateLegacy();
    const id = serverId ?? getActiveServerId();
    if (!id) return false;
    const profile = getServer(id);
    if (!profile) return false;
    const persisted = readPersisted()[id];
    if (!persisted) return false;
    try {
        const pair = await postJSON<TokenPairResponse>(
            profile.url + "/api/v1/auth/refresh",
            { refresh_token: persisted.refreshToken },
        );
        storeTokens(profile, pair, persisted.user);
        return true;
    } catch {
        // refresh token rejected — drop it so the rail can surface
        // "expired" state and the user isn't stuck in a retry loop.
        sessions.delete(id);
        dropPersisted(id);
        emit();
        return false;
    }
}

// switchServer flips the active pointer, opportunistically refreshing
// the target server's session from its cached refresh token. Returns
// whether the target is now logged in; callers route to /login with
// serverId state when `loggedIn=false`.
export async function switchServer(id: string): Promise<{ loggedIn: boolean }> {
    setActiveServerId(id);
    if (sessions.has(id)) {
        return { loggedIn: true };
    }
    const ok = await refresh(id);
    return { loggedIn: ok };
}

// forgetServer drops the in-memory session and the persisted refresh
// token for one server. The ServerProfile itself stays — "Sign out"
// in Manage Servers uses this; "Remove server" also calls through to
// servers.removeServer() afterwards.
export function forgetServer(id: string): void {
    sessions.delete(id);
    dropPersisted(id);
    emit();
}

export function forgetAndRemoveServer(id: string): void {
    // Flip the active pointer FIRST (inside removeServer) so
    // RequireAuth's session check doesn't observe "active=X but
    // session=null" between the two mutations and bounce the user
    // to /login. Dropping the stored tokens is the last step.
    removeServer(id);
    forgetServer(id);
}

// --- Change password / logout --------------------------------------

export async function changePassword(
    oldPassword: string,
    newPassword: string,
): Promise<void> {
    await authFetch("/api/v1/auth/password", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
    const id = getActiveServerId();
    if (id) {
        sessions.delete(id);
        dropPersisted(id);
    }
    emit();
}

export async function logout(): Promise<void> {
    const s = getSession();
    if (!s) return;
    const url = s.serverURL + "/api/v1/auth/logout";
    const token = s.refreshToken;
    sessions.delete(s.serverId);
    dropPersisted(s.serverId);
    emit();
    try {
        await postJSON(url, { refresh_token: token });
    } catch {
        // ignore — local state is already cleared
    }
}

// --- authFetch ------------------------------------------------------

// StaleServerResponseError is thrown when a fetch resolves after the
// user has switched to a different server. Pages that subscribe to
// onActiveChange re-fetch on their own; this error keeps a stray
// response from contaminating the new view.
export class StaleServerResponseError extends Error {
    constructor(public serverIdAtCall: string, public activeServerIdNow: string | null) {
        super(`response for server ${serverIdAtCall} arrived after switch to ${activeServerIdNow ?? "none"}`);
    }
}

export async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
    const id = getActiveServerId();
    if (!id) throw new Error("not logged in");
    let session = sessions.get(id);
    if (!session) {
        // No live access token for the active server — try one
        // synchronous refresh before giving up.
        const ok = await refresh(id);
        if (!ok) throw new Error("not logged in");
        session = sessions.get(id)!;
    }

    const doFetch = async (s: Session) => {
        const headers = new Headers(init.headers);
        headers.set("Authorization", "Bearer " + s.accessToken);
        return fetch(s.serverURL + path, { ...init, headers });
    };

    let r = await doFetch(session);
    if (r.status === 401) {
        const ok = await refresh(id);
        if (!ok) throw new Error("session expired");
        const next = sessions.get(id);
        if (!next) throw new Error("session expired");
        r = await doFetch(next);
    }

    // Guard against cross-server races: if the user switched while the
    // fetch was in flight, reject so the caller doesn't merge the
    // stale response into the new server's state.
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

// probeServer hits the authless /api/v1/auth/info endpoint so the
// onboarding wizard and AddServerDialog can tell users whether to log
// in or run the first-time-setup flow before they type a password.
export async function probeServer(url: string): Promise<PublicServerInfo> {
    const base = normaliseURL(url);
    const r = await fetch(base + "/api/v1/auth/info");
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
    return (await r.json()) as PublicServerInfo;
}
