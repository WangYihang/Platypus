// Per-server unread aggregator. Subscribes once to notify fan-out
// events that matter across workspaces (new session, host seen) and
// tracks a count per serverId. The ServerRail consumes counts via
// useUnread(id); clears happen on switchServer.

import { getActiveServerId, onServersChange } from "./servers";
import { NotifyEvent, onNotifyAny } from "./notify";
import { onActiveChange } from "./auth";
import { useSyncExternalStore } from "react";

const counts = new Map<string, number>();
const listeners = new Set<() => void>();

function emit() {
    for (const fn of listeners) {
        try {
            fn();
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("unread listener threw:", err);
        }
    }
}

function bump(serverId: string) {
    if (serverId === getActiveServerId()) return;
    counts.set(serverId, (counts.get(serverId) ?? 0) + 1);
    emit();
}

function clear(serverId: string | null) {
    if (!serverId) return;
    if (!counts.has(serverId)) return;
    counts.delete(serverId);
    emit();
}

// One-time wiring. Module import order guarantees this runs before
// any component reads useUnread.
onNotifyAny(NotifyEvent.SessionOpened, (ev) => bump(ev.serverId));
onNotifyAny(NotifyEvent.HostSeen, (ev) => bump(ev.serverId));

// Clear on switch — the user "saw" anything queued on this server by
// focusing it.
onActiveChange(() => {
    clear(getActiveServerId());
});

// Clear on remove so orphan counters don't linger.
onServersChange(() => {
    const current = getActiveServerId();
    if (current) clear(current);
});

function subscribe(fn: () => void): () => void {
    listeners.add(fn);
    return () => {
        listeners.delete(fn);
    };
}

function snapshot(serverId: string): number {
    return counts.get(serverId) ?? 0;
}

// useUnread is a tiny useSyncExternalStore hook so ServerTile stays a
// pure function of its props. The `getServerSnapshot` branch is the
// same as client since we only run in the browser.
export function useUnread(serverId: string): number {
    return useSyncExternalStore(
        subscribe,
        () => snapshot(serverId),
        () => snapshot(serverId),
    );
}

export function peekUnread(serverId: string): number {
    return snapshot(serverId);
}
