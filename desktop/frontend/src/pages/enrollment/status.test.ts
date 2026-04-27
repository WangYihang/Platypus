import { describe, expect, it } from "vitest";

import { STATUS_LABEL, STATUS_TONE } from "./status";

// status.ts is the single source of truth for enrollment-status
// vocabulary. The labels (Unused / Used / Expired / Revoked) abstract
// the back-end's lifecycle words so the screen reads in user terms,
// and the tones drive the StatusPill colour. The pinning here is:
//
//   · "pending" is "ready to be consumed" — it must NOT read as an
//     inert/disabled neutral pill, which is what the previous gray
//     looked like. Operators saw "Unused" + gray and concluded the
//     install command was stale; switching to info (blue) signals
//     "ready, waiting for the agent".
//
//   · The other tones encode finality (success / warning / danger)
//     and are unchanged from before the split.

describe("enrollment status maps", () => {
    it("labels each status with the user-friendly word", () => {
        expect(STATUS_LABEL.pending).toBe("Unused");
        expect(STATUS_LABEL.consumed).toBe("Used");
        expect(STATUS_LABEL.expired).toBe("Expired");
        expect(STATUS_LABEL.revoked).toBe("Revoked");
    });

    it("colours pending as info — NOT neutral — so 'ready' reads as ready", () => {
        expect(STATUS_TONE.pending).toBe("info");
    });

    it("keeps consumed=success, expired=warning, revoked=danger", () => {
        expect(STATUS_TONE.consumed).toBe("success");
        expect(STATUS_TONE.expired).toBe("warning");
        expect(STATUS_TONE.revoked).toBe("danger");
    });
});
