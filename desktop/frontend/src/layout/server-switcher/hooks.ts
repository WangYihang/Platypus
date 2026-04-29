import { useEffect, useMemo, useState, useSyncExternalStore } from "react";

import { ServerProfile, useServersStore } from "../../lib/servers";
import {
    ConnectionState,
    connectionState as connState,
    onConnectionStateChange,
} from "../../lib/notify";
import { hasPersistedSession, onSessionChange } from "../../lib/auth";

// Plain zustand selectors. Selectors return arrays (compared by reference;
// zustand's default Object.is is fine because we always replace the array
// on mutation) or primitive strings — both stable.

export function useServerList(): ServerProfile[] {
    return useServersStore((s) => s.profiles);
}

export function useActiveServerId(): string | null {
    return useServersStore((s) => s.activeId);
}

export function useActiveServer(): ServerProfile | null {
    return useServersStore((s) =>
        s.activeId ? s.profiles.find((p) => p.id === s.activeId) ?? null : null,
    );
}

export function useConnectionState(serverId: string): ConnectionState {
    const [state, setState] = useState(() => connState(serverId));
    useEffect(() => {
        return onConnectionStateChange((id, s) => {
            if (id === serverId) setState(s);
        });
    }, [serverId]);
    return state;
}

let sessionVersion = 0;
onSessionChange(() => {
    sessionVersion++;
});

export function useHasPersistedSession(serverId: string): boolean {
    useSyncExternalStore(
        (fn) => onSessionChange(fn),
        () => sessionVersion,
        () => sessionVersion,
    );
    return useMemo(
        () => hasPersistedSession(serverId),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [serverId, sessionVersion],
    );
}
