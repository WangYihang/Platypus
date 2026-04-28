// autoOpenShell — pure decision helper for HostView.
//
// HostView wants to open a terminal automatically the first time the
// operator lands on a host that's actually reachable, since the
// motivating UX is "I clicked a host because I want a shell". We
// only ever fire once per visit and we never overwrite a shell the
// operator already has open for this host.
//
// The hook in HostView.tsx wraps this with refs / openShell calls.
// The decision itself is pure so the contract can be pinned with a
// trivial unit test, no React renderer required.

export type AutoOpenAction =
    | { kind: "open" }
    | { kind: "mark" }
    | { kind: "skip" };

export interface AutoOpenInputs {
    /** Has this hook already fired in the current visit? Tracked
     *  in a ref so re-renders don't re-open a shell the operator
     *  closed deliberately. */
    alreadyAutoOpened: boolean;
    /** Did the host row resolve far enough to have an agent_id?
     *  Without one the shell URL has no session_hash. */
    hasAgentID: boolean;
    /** At least one session row has disconnected_at == null. */
    hasLiveSession: boolean;
    /** A shell for this host is already open in the global drawer
     *  (e.g. operator switched to a different host and came back).
     *  We mark the auto-open as "done" so we won't fire on a future
     *  re-render, but we won't open a duplicate shell. */
    shellAlreadyOpenForHost: boolean;
}

// decideAutoOpenShell answers the three-state question the hook asks
// every time the relevant inputs change.
export function decideAutoOpenShell(input: AutoOpenInputs): AutoOpenAction {
    if (input.alreadyAutoOpened) return { kind: "skip" };
    if (!input.hasAgentID) return { kind: "skip" };
    if (!input.hasLiveSession) return { kind: "skip" };
    if (input.shellAlreadyOpenForHost) return { kind: "mark" };
    return { kind: "open" };
}
