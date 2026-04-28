// Per-server notify pool. Each saved ServerProfile gets its own
// WebSocket to /notify; subscribers can bind to "the active server"
// (auto-rebinds on switchServer), to a specific serverId, or to
// "every server" for cross-workspace unread aggregation.
//
// The previous single-socket design is preserved behaviourally for
// existing callers (HostView, Fleet, StatusBar): `onNotify(type, fn)`
// without a serverId follows the active pointer.

import {
    authFetch,
    forgetServer,
    getSession,
    onActiveChange,
    onSessionChange,
} from "./auth";
import { getActiveServerId, getServer, onServersChange } from "./servers";

type Listener = (data: unknown) => void;
type AnyListener = (ev: { serverId: string; data: unknown }) => void;

export type ConnectionState =
    | "idle"
    | "connecting"
    | "connected"
    | "closed"
    | "errored";

interface Connection {
    serverId: string;
    socket: WebSocket | null;
    connecting: Promise<void> | null;
    listeners: Map<string, Set<Listener>>;
    state: ConnectionState;
}

const pool = new Map<string, Connection>();

// activeBoundListeners are subscriptions that track the active
// server pointer. On every switch we re-bind them to the new
// server's Connection.
interface ActiveBinding {
    type: string;
    fn: Listener;
    currentServerId: string | null;
}
const activeBindings = new Set<ActiveBinding>();

// anyListeners fan every connection's messages out by type so the
// rail's unread aggregator can count across servers without wiring
// N subscriptions manually.
const anyListeners = new Map<string, Set<AnyListener>>();

// connectionStateListeners observe state transitions on any
// Connection. The rail tile's status dot subscribes here.
type ConnectionStateListener = (serverId: string, state: ConnectionState) => void;
const connectionStateListeners = new Set<ConnectionStateListener>();

function emitConnectionState(serverId: string, state: ConnectionState) {
    for (const fn of connectionStateListeners) {
        try {
            fn(serverId, state);
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("connection state listener threw:", err);
        }
    }
}

export function onConnectionStateChange(fn: ConnectionStateListener): () => void {
    connectionStateListeners.add(fn);
    return () => {
        connectionStateListeners.delete(fn);
    };
}

export function connectionState(serverId: string): ConnectionState {
    return pool.get(serverId)?.state ?? "idle";
}

function setState(conn: Connection, state: ConnectionState) {
    if (conn.state === state) return;
    conn.state = state;
    emitConnectionState(conn.serverId, state);
}

function getOrCreateConnection(serverId: string): Connection {
    let c = pool.get(serverId);
    if (c) return c;
    c = {
        serverId,
        socket: null,
        connecting: null,
        listeners: new Map(),
        state: "idle",
    };
    pool.set(serverId, c);
    return c;
}

// Mint a one-shot WS ticket via the authenticated REST path; the
// server rejects /notify upgrades without one.
async function issueTicket(): Promise<string> {
    const r = await authFetch("/api/v1/ws/ticket", { method: "POST" });
    const j = (await r.json()) as { ticket: string };
    return j.ticket;
}

function wsURL(httpURL: string, ticket: string): string {
    const u = new URL(httpURL);
    u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
    u.pathname = "/notify";
    u.search = `?ticket=${encodeURIComponent(ticket)}`;
    return u.toString();
}

async function connect(conn: Connection): Promise<void> {
    if (conn.socket && conn.socket.readyState === WebSocket.OPEN) return;
    if (conn.connecting) return conn.connecting;

    const profile = getServer(conn.serverId);
    const session = getSession(conn.serverId);
    if (!profile || !session) return;

    setState(conn, "connecting");
    conn.connecting = (async () => {
        // The ticket endpoint is auth-gated; authFetch picks the
        // active server, so we temporarily flip active for the
        // ticket call. In practice callers only open a non-active
        // connection via onNotifyAny, which is fine here because
        // we're inside its first-tick setup.
        const prevActive = getActiveServerId();
        let ticket: string;
        try {
            const wasActive = prevActive === conn.serverId;
            if (!wasActive) {
                // Fetch via raw REST with this server's token, not
                // via authFetch — flipping the active pointer would
                // cascade unrelated re-renders.
                const r = await fetch(profile.url + "/api/v1/ws/ticket", {
                    method: "POST",
                    headers: { Authorization: "Bearer " + session.sessionToken },
                });
                if (!r.ok) throw new Error(`ticket: ${r.status}`);
                const j = (await r.json()) as { ticket: string };
                ticket = j.ticket;
            } else {
                ticket = await issueTicket();
            }
        } catch (err) {
            setState(conn, "errored");
            conn.connecting = null;
            throw err;
        }

        const url = wsURL(profile.url, ticket);
        const ws = new WebSocket(url);
        conn.socket = ws;

        try {
            await new Promise<void>((resolve, reject) => {
                ws.addEventListener("open", () => resolve(), { once: true });
                ws.addEventListener("error", () => reject(new Error("ws error")), {
                    once: true,
                });
            });
        } catch (err) {
            setState(conn, "errored");
            conn.connecting = null;
            conn.socket = null;
            throw err;
        }

        setState(conn, "connected");

        ws.addEventListener("message", (e) => {
            try {
                const env = JSON.parse(String(e.data)) as {
                    type?: string;
                    data?: unknown;
                };
                if (!env.type) return;

                // Per-type listeners scoped to this Connection.
                const set = conn.listeners.get(env.type);
                if (set) {
                    for (const l of set) {
                        try {
                            l(env.data);
                        } catch (err) {
                            // eslint-disable-next-line no-console
                            console.error("notify listener threw:", err);
                        }
                    }
                }

                // Fan to onNotifyAny subscribers.
                const any = anyListeners.get(env.type);
                if (any) {
                    for (const l of any) {
                        try {
                            l({ serverId: conn.serverId, data: env.data });
                        } catch (err) {
                            // eslint-disable-next-line no-console
                            console.error("notify any-listener threw:", err);
                        }
                    }
                }
            } catch {
                // unparseable frame — ignore
            }
        });

        ws.addEventListener("close", () => {
            conn.socket = null;
            conn.connecting = null;
            setState(conn, "closed");
        });
    })();

    try {
        await conn.connecting;
    } finally {
        conn.connecting = null;
    }
}

function disconnect(serverId: string): void {
    const conn = pool.get(serverId);
    if (!conn) return;
    if (conn.socket) {
        try {
            conn.socket.close();
        } catch {
            // ignore
        }
        conn.socket = null;
    }
    conn.connecting = null;
    setState(conn, "closed");
}

// Public API ----------------------------------------------------------

export interface OnNotifyOptions {
    // serverId=undefined → follow the active pointer (re-binds
    // automatically on switchServer). Pass an id to pin to a
    // specific server regardless of active.
    serverId?: string;
}

export function onNotify(
    eventType: string,
    fn: Listener,
    opts: OnNotifyOptions = {},
): () => void {
    if (opts.serverId) {
        return bindOne(opts.serverId, eventType, fn);
    }

    // Active-bound: rebind on switch.
    const binding: ActiveBinding = {
        type: eventType,
        fn,
        currentServerId: null,
    };
    activeBindings.add(binding);

    const rebind = () => {
        const next = getActiveServerId();
        if (binding.currentServerId === next) return;
        if (binding.currentServerId) {
            removeListener(binding.currentServerId, binding.type, binding.fn);
        }
        binding.currentServerId = next;
        if (next) {
            addListener(next, binding.type, binding.fn);
        }
    };
    rebind();

    return () => {
        if (binding.currentServerId) {
            removeListener(binding.currentServerId, binding.type, binding.fn);
        }
        activeBindings.delete(binding);
    };
}

export function onNotifyAny(
    eventType: string,
    fn: AnyListener,
): () => void {
    let set = anyListeners.get(eventType);
    if (!set) {
        set = new Set();
        anyListeners.set(eventType, set);
    }
    set.add(fn);

    // Ensure every server with a live session is connected so we
    // capture events from inactive workspaces too.
    for (const id of poolableServerIds()) {
        void connectIfPossible(id);
    }

    return () => {
        const s = anyListeners.get(eventType);
        if (!s) return;
        s.delete(fn);
        if (s.size === 0) anyListeners.delete(eventType);
    };
}

function bindOne(serverId: string, type: string, fn: Listener): () => void {
    addListener(serverId, type, fn);
    return () => removeListener(serverId, type, fn);
}

function addListener(serverId: string, type: string, fn: Listener): void {
    const conn = getOrCreateConnection(serverId);
    let set = conn.listeners.get(type);
    if (!set) {
        set = new Set();
        conn.listeners.set(type, set);
    }
    set.add(fn);
    void connectIfPossible(serverId);
}

function removeListener(serverId: string, type: string, fn: Listener): void {
    const conn = pool.get(serverId);
    if (!conn) return;
    const set = conn.listeners.get(type);
    if (!set) return;
    set.delete(fn);
    if (set.size === 0) conn.listeners.delete(type);
}

async function connectIfPossible(serverId: string): Promise<void> {
    const conn = getOrCreateConnection(serverId);
    if (conn.socket || conn.connecting) return;
    try {
        await connect(conn);
    } catch {
        // Errors surface via setState("errored"); swallow here so
        // the subscription itself never rejects.
    }
}

function poolableServerIds(): string[] {
    // Only servers that have a live session can be connected.
    // Everything else will just sit in "closed"/"errored" until the
    // user logs into them.
    const ids: string[] = [];
    for (const id of pool.keys()) ids.push(id);
    // Also include any server that currently has a session even if
    // not yet in the pool.
    // (We can't enumerate sessions.keys() cheaply without exposing
    // them; rely on getSession per-id when we spin connections up
    // for new servers.)
    return ids.length > 0 ? ids : activeServerIdIfAny();
}

function activeServerIdIfAny(): string[] {
    const id = getActiveServerId();
    return id ? [id] : [];
}

// Re-bind active listeners when the pointer flips.
onActiveChange(() => {
    for (const binding of activeBindings) {
        const next = getActiveServerId();
        if (binding.currentServerId === next) continue;
        if (binding.currentServerId) {
            removeListener(binding.currentServerId, binding.type, binding.fn);
        }
        binding.currentServerId = next;
        if (next) {
            addListener(next, binding.type, binding.fn);
        }
    }
});

// If a server loses its session (forgetServer / refresh failure),
// close its socket so the tile goes gray.
onSessionChange(() => {
    for (const id of pool.keys()) {
        const s = getSession(id);
        if (!s) disconnect(id);
    }
});

// If a server profile is removed, tear its connection down entirely.
onServersChange(() => {
    for (const id of pool.keys()) {
        if (!getServer(id)) {
            disconnect(id);
            pool.delete(id);
        }
    }
});

// disconnectServer is exported so higher-level code (auth.forgetServer
// in a future migration) can explicitly close a tile's socket without
// waiting for the next onSessionChange tick.
export function disconnectServer(serverId: string): void {
    disconnect(serverId);
    forgetServer; // reference to keep import live across tree-shaking
}

// --- Event constants ------------------------------------------------

export const NotifyEvent = {
    HostSeen: "host.seen",
    SessionOpened: "session.opened",
    SessionClosed: "session.closed",
    TopologyLinkUp: "topology.link_up",
    TopologyLinkDown: "topology.link_down",
    TopologyLinkStats: "topology.link_stats",
    TopologyMachineStats: "topology.machine_stats",
    TopologyNodeJoined: "topology.node_joined",
    TopologyNodeLeft: "topology.node_left",
    FileTransferUpdated: "file_transfer_updated",
} as const;

export interface HostSeenPayload {
    project_id: string;
    host_id: string;
    hostname?: string;
    fingerprint_fallback?: boolean;
}

export interface SessionEventPayload {
    project_id: string;
    host_id: string;
    session_id: string;
}

// --- Topology event payloads ----------------------------------------

export interface TopologyLinkUpPayload {
    project_id: string;
    peer: string;
    remote_addr: string;
    at: string;
}

export interface TopologyLinkDownPayload {
    project_id: string;
    peer: string;
    at: string;
}

export interface TopologyLinkStatsEntry {
    a: string;
    b: string;
    rtt_ns: number;
    bytes_in: number;
    bytes_out: number;
    msgs_in: number;
    msgs_out: number;
}

export interface TopologyLinkStatsPayload {
    project_id: string;
    tick_at: string;
    links: TopologyLinkStatsEntry[];
}

export interface TopologyMachineStatsPayload {
    project_id: string;
    host_id: string;
    cpu_percent: number;
    mem_percent: number;
    sampled_at: number;
    sys_info?: {
        kernel_version?: string;
        os_distribution?: string;
        cpu_percent?: number;
        mem_percent?: number;
        mem_total_bytes?: number;
        mem_used_bytes?: number;
        uptime_seconds?: number;
    };
}

export interface TopologyNodeEventPayload {
    project_id: string;
    node_id: string;
    machine_id?: string;
}
