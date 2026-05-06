// HostContext shares the per-host data that lives in HostView with
// the activity panes underneath, so first-party tabs migrated to the
// PLUGIN_UI_REGISTRY (Files, Info, Sessions, …) can pick up `host`,
// `sysInfo`, `sessions`, etc. without HostView passing them through
// the registry's PluginUIProps shape.
//
// Why a context (not extra PluginUIProps fields):
//   - PluginUIProps is the contract every plugin tab component sees;
//     keeping it minimal (projectID / agentID / hostOS / active) lets
//     pure plugin authors stay decoupled from the host-page chrome.
//   - First-party-tab adapters opt into this richer view via the
//     `useHostContext()` hook below; out-of-tree plugins just don't
//     call it.

import { createContext, ReactNode, useContext } from "react";

import type { Host, HostSysInfo, SessionRow } from "../../lib/api";

export interface HostContextValue {
    host: Host;
    /**
     * The host's server-side primary key (UUID). Distinct from
     * agentID (cert-derived public id used for the wire). Several
     * server routes key off hostID rather than agentID, so the
     * first-party-tab adapters need both.
     */
    hostID: string;
    sessions: ReadonlyArray<SessionRow>;
    /** Agent_id when there's at least one live session, null otherwise. */
    pickedSessionID: string | null;
    sysInfo: HostSysInfo | null;
    sysInfoError: string | null;
    sysInfoLoading: boolean;
    refreshSysInfo: () => void;
}

const HostContextInternal = createContext<HostContextValue | null>(null);

export function HostContextProvider({
    value,
    children,
}: {
    value: HostContextValue;
    children: ReactNode;
}) {
    return (
        <HostContextInternal.Provider value={value}>
            {children}
        </HostContextInternal.Provider>
    );
}

/**
 * Reads the current host context. Throws when called outside a
 * HostContextProvider — first-party-tab adapters are mounted by
 * HostView which always wraps in the provider, so a missing context
 * is a programming error, not a runtime branch.
 */
export function useHostContext(): HostContextValue {
    const ctx = useContext(HostContextInternal);
    if (!ctx) {
        throw new Error(
            "useHostContext must be used inside <HostContextProvider>",
        );
    }
    return ctx;
}
