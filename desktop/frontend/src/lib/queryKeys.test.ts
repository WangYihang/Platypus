import { describe, expect, it } from "vitest";

import { qk } from "./queryKeys";

// queryKeys is the single source of truth for every react-query
// key tuple. The shapes pinned here are what
// `queryClient.invalidateQueries({ queryKey: qk.X(...) })` matches
// against, so a typo in one consumer would silently break
// invalidation. These specs catch that at compile time (TS) and
// at runtime (structural equality).

describe("qk — query-key registry", () => {
    it("hosts is keyed by projectId only", () => {
        expect(qk.hosts("p1")).toEqual(["hosts", "p1"]);
        // Different project = different cache slot.
        expect(qk.hosts("p1")).not.toEqual(qk.hosts("p2"));
    });

    it("host / hostSysInfo / hostSessions / hostProcesses partition by hostId", () => {
        const a = qk.host("p1", "h1");
        const b = qk.hostSysInfo("p1", "h1");
        const c = qk.hostSessions("p1", "h1");
        const d = qk.hostProcesses("p1", "h1");
        // First segment is the resource type — the discriminator
        // react-query uses for partial-match invalidation.
        expect(a[0]).toBe("host");
        expect(b[0]).toBe("hostSysInfo");
        expect(c[0]).toBe("hostSessions");
        expect(d[0]).toBe("hostProcesses");
    });

    it("activities embeds the opts object so different filters key separately", () => {
        const k1 = qk.activities("p1", { sources: ["human"] });
        const k2 = qk.activities("p1", { sources: ["system"] });
        expect(k1).not.toEqual(k2);
        // Same opts → same key (shallow structural equality).
        expect(qk.activities("p1", { sources: ["human"] })).toEqual(k1);
    });

    it("project-less and admin keys are constant", () => {
        expect(qk.projects()).toEqual(["projects"]);
        expect(qk.adminUsers()).toEqual(["adminUsers"]);
        expect(qk.serverInfo()).toEqual(["serverInfo"]);
    });
});
