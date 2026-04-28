import { describe, expect, it } from "vitest";

import { basename, formatBytes, formatUptimeSeconds } from "./format";

// formatBytes / formatUptimeSeconds back the status-bar telemetry
// pills. The contract is mostly UX:
//
//   · formatBytes returns a short human string ("47 MiB", "1.2 GiB").
//     1024-based — the status bar runs against runtime.MemStats.Alloc
//     which counts in binary units, so "MB" would be misleading.
//   · formatUptimeSeconds returns "Nd Mh", "Mh Ks", or "Ss" — the
//     two most-significant units, capped so the pill stays readable.

describe("formatBytes", () => {
    it("returns 0 B for zero", () => {
        expect(formatBytes(0)).toBe("0 B");
    });

    it("returns whole bytes below 1 KiB", () => {
        expect(formatBytes(900)).toBe("900 B");
    });

    it("returns KiB / MiB / GiB / TiB with one decimal", () => {
        expect(formatBytes(1024)).toBe("1.0 KiB");
        expect(formatBytes(2048)).toBe("2.0 KiB");
        expect(formatBytes(47 * 1024 * 1024)).toBe("47.0 MiB");
        expect(formatBytes(1.5 * 1024 * 1024 * 1024)).toBe("1.5 GiB");
    });

    it("renders nullish input as a dash so callers can splice unchecked", () => {
        expect(formatBytes(null)).toBe("—");
        expect(formatBytes(undefined)).toBe("—");
    });
});

describe("formatUptimeSeconds", () => {
    it("seconds for very fresh processes", () => {
        expect(formatUptimeSeconds(5)).toBe("5s");
        expect(formatUptimeSeconds(45)).toBe("45s");
    });

    it("minutes and seconds under an hour", () => {
        expect(formatUptimeSeconds(60)).toBe("1m");
        expect(formatUptimeSeconds(125)).toBe("2m 5s");
    });

    it("hours and minutes under a day", () => {
        expect(formatUptimeSeconds(60 * 60)).toBe("1h");
        expect(formatUptimeSeconds(2 * 60 * 60 + 30 * 60)).toBe("2h 30m");
    });

    it("days and hours otherwise", () => {
        expect(formatUptimeSeconds(86400)).toBe("1d");
        expect(formatUptimeSeconds(3 * 86400 + 4 * 3600)).toBe("3d 4h");
    });

    it("renders nullish / negative input as a dash", () => {
        expect(formatUptimeSeconds(null)).toBe("—");
        expect(formatUptimeSeconds(undefined)).toBe("—");
        expect(formatUptimeSeconds(-1)).toBe("—");
    });
});

describe("basename (regression)", () => {
    // basename is unchanged by this commit but lives in the same
    // file; pin its existing behaviour so a test refactor doesn't
    // accidentally drop it.
    it("returns the leaf segment", () => {
        expect(basename("/etc/nginx/nginx.conf")).toBe("nginx.conf");
        expect(basename("/")).toBe("");
        expect(basename("nginx.conf")).toBe("nginx.conf");
    });
});
