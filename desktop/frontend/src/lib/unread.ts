// Per-server unread aggregator. Subscribes once to notify fan-out
// events that matter across workspaces (new session, host seen) and
// tracks a count per serverId. ServerSwitcher consumes counts via
// `useUnread(id)`; clears happen on switchServer.
//
// Backed by a zustand store (replaced the previous hand-rolled
// `Map<serverId, count> + Set<listener> + emit()` registry). The
// counts slot is an immutable Record so zustand selectors stay
// referentially stable per-server until that server's count
// changes.

import { create } from "zustand";

import { getActiveServerId, onServersChange } from "./servers";
import { NotifyEvent, onNotifyAny } from "./notify";
import { onActiveChange } from "./auth";

interface UnreadState {
    counts: Record<string, number>;
}

const useUnreadStore = create<UnreadState>(() => ({
    counts: {},
}));

function bump(serverId: string) {
    if (serverId === getActiveServerId()) return;
    useUnreadStore.setState((s) => {
        const current = s.counts[serverId] ?? 0;
        return { counts: { ...s.counts, [serverId]: current + 1 } };
    });
}

function clear(serverId: string | null) {
    if (!serverId) return;
    useUnreadStore.setState((s) => {
        if (!(serverId in s.counts)) return s;
        const { [serverId]: _drop, ...rest } = s.counts;
        return { counts: rest };
    });
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

// useUnread selects this server's count out of the store. Selector
// returns a primitive number, so zustand's default Object.is
// comparison is exactly right — no derivation pitfalls.
export function useUnread(serverId: string): number {
    return useUnreadStore((s) => s.counts[serverId] ?? 0);
}

export function peekUnread(serverId: string): number {
    return useUnreadStore.getState().counts[serverId] ?? 0;
}
