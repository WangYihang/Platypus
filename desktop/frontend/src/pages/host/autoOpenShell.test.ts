import { describe, expect, it } from "vitest";

import { decideAutoOpenShell } from "./autoOpenShell";

const base = {
    alreadyAutoOpened: false,
    hasAgentID: true,
    hasLiveSession: true,
    shellAlreadyOpenForHost: false,
};

// The pure helper is the contract for the auto-open behaviour: HostView
// opens a shell on the first navigation to a reachable host, and never
// stomps a shell the operator already has open.
describe("decideAutoOpenShell", () => {
    it("opens a fresh shell on first visit when the host is reachable", () => {
        expect(decideAutoOpenShell(base)).toEqual({ kind: "open" });
    });

    it("skips when the hook has already fired in this visit", () => {
        expect(
            decideAutoOpenShell({ ...base, alreadyAutoOpened: true }),
        ).toEqual({ kind: "skip" });
    });

    it("skips while the host row has no agent_id (still loading)", () => {
        expect(decideAutoOpenShell({ ...base, hasAgentID: false })).toEqual({
            kind: "skip",
        });
    });

    it("skips when no live session is present (agent disconnected)", () => {
        expect(decideAutoOpenShell({ ...base, hasLiveSession: false })).toEqual({
            kind: "skip",
        });
    });

    it("marks (no second shell) when a shell for this host is already open", () => {
        expect(
            decideAutoOpenShell({ ...base, shellAlreadyOpenForHost: true }),
        ).toEqual({ kind: "mark" });
    });
});
