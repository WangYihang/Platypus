// Single WebSocket connection to /notify that fans events out to
// subscribers by type. One shared socket across the tab — every
// Sidebar / HostView / ListenerView subscriber reuses the same
// underlying ws.Handle.
//
// The socket is reconnected once lazily on first subscribe. Deeper
// survival work (exponential backoff, gap-fill on reconnect) is
// deliberately deferred per the T1.2 open questions section of the
// plan — we'll observe real-world behaviour first.

import { authFetch, getSession, onSessionChange } from "./auth";

type Listener = (data: unknown) => void;

const listeners = new Map<string, Set<Listener>>();
let socket: WebSocket | null = null;
let connecting: Promise<void> | null = null;
let sessionUnsub: (() => void) | null = null;

// Mint a one-shot WS ticket via the authenticated REST path so the
// browser can pass ?ticket= on the upgrade URL. The server rejects
// upgrades without one-shot auth.
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

function disconnect() {
    if (sessionUnsub) {
        sessionUnsub();
        sessionUnsub = null;
    }
    if (socket) {
        socket.close();
        socket = null;
    }
    connecting = null;
}

async function connect(): Promise<void> {
    if (socket && socket.readyState === WebSocket.OPEN) return;
    if (connecting) return connecting;

    const s = getSession();
    if (!s) throw new Error("no session; cannot open notify socket");

    connecting = (async () => {
        const ticket = await issueTicket();
        const url = wsURL(s.serverURL, ticket);
        const ws = new WebSocket(url);
        socket = ws;

        await new Promise<void>((resolve, reject) => {
            ws.addEventListener("open", () => resolve(), { once: true });
            ws.addEventListener("error", () => reject(new Error("ws error")), {
                once: true,
            });
        });

        ws.addEventListener("message", (e) => {
            try {
                const env = JSON.parse(String(e.data)) as { type?: string; data?: unknown };
                if (!env.type) return;
                const ls = listeners.get(env.type);
                if (!ls) return;
                for (const l of ls) {
                    try {
                        l(env.data);
                    } catch (err) {
                        // eslint-disable-next-line no-console
                        console.error("notify listener threw:", err);
                    }
                }
            } catch {
                // unparseable frame — ignore
            }
        });
        ws.addEventListener("close", () => {
            socket = null;
            connecting = null;
        });

        // If the session is torn down (logout, refresh failure), drop
        // the socket so a future login starts fresh.
        if (!sessionUnsub) {
            sessionUnsub = onSessionChange(() => {
                if (!getSession()) disconnect();
            });
        }
    })();

    return connecting;
}

// onNotify subscribes `fn` to events of the given type. Returns an
// unsubscribe function. First call triggers a lazy connect; later calls
// piggyback on the existing socket.
export function onNotify(eventType: string, fn: Listener): () => void {
    let set = listeners.get(eventType);
    if (!set) {
        set = new Set();
        listeners.set(eventType, set);
    }
    set.add(fn);

    // Fire off a connect if we don't have one yet. Errors are swallowed
    // — subscribers should call their own initial refetch regardless
    // to cover the gap between mount and socket-open.
    void connect().catch(() => {});

    return () => {
        const ls = listeners.get(eventType);
        if (!ls) return;
        ls.delete(fn);
        if (ls.size === 0) listeners.delete(eventType);
    };
}

// Event name constants mirror the Go core.Event* set. Importing these
// instead of literal strings catches typos at compile time.
export const NotifyEvent = {
    HostSeen: "host.seen",
    SessionOpened: "session.opened",
    SessionClosed: "session.closed",
    ListenerCreated: "listener.created",
    ListenerDeleted: "listener.deleted",
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

export interface ListenerEventPayload {
    project_id: string;
    listener_id: string;
    host?: string;
    port?: number;
}
