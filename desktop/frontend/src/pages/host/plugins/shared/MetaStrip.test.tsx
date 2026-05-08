// Direct tests for the shared MetaStrip module. The 4 integration
// tests in RPCTable.test.tsx already exercise MetaStrip via its
// only original consumer; these add unit coverage so callers that
// don't go through RPCTable (Processes / Config / Security tabs)
// rely on tested primitives.

import { describe, expect, it, beforeEach } from "vitest";
import { fireEvent, render, renderHook, screen } from "@testing-library/react";

import {
    MetaStrip,
    formatRelativeAge,
    useRefreshInterval,
} from "./MetaStrip";

beforeEach(() => {
    try {
        window.localStorage.clear();
    } catch {
        // ignore
    }
});

describe("formatRelativeAge", () => {
    it("returns 'just now' under 5s", () => {
        expect(formatRelativeAge(0)).toBe("just now");
        expect(formatRelativeAge(4_999)).toBe("just now");
    });

    it("returns 'Xs ago' for seconds-scale ages", () => {
        expect(formatRelativeAge(12_000)).toBe("12s ago");
        expect(formatRelativeAge(59_000)).toBe("59s ago");
    });

    it("returns 'Xm ago' for minute-scale ages", () => {
        expect(formatRelativeAge(60_000)).toBe("1m ago");
        expect(formatRelativeAge(180_000)).toBe("3m ago");
    });

    it("returns 'Xh ago' for hour-scale ages", () => {
        expect(formatRelativeAge(3_600_000)).toBe("1h ago");
        expect(formatRelativeAge(7_200_000)).toBe("2h ago");
    });
});

describe("useRefreshInterval", () => {
    it("falls back to defaultMs when nothing is persisted", () => {
        const { result } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-A", 5000),
        );
        expect(result.current.effectiveMs).toBe(5000);
    });

    it("reads a persisted value on mount", () => {
        window.localStorage.setItem(
            "rpc-refresh:plug.X:agent-A",
            "30000",
        );
        const { result } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-A", 5000),
        );
        expect(result.current.effectiveMs).toBe(30000);
    });

    it("a persisted 0 (Off) survives even when default is non-zero", () => {
        window.localStorage.setItem("rpc-refresh:plug.X:agent-A", "0");
        const { result } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-A", 5000),
        );
        expect(result.current.effectiveMs).toBe(0);
    });

    it("ignores corrupt persisted values", () => {
        window.localStorage.setItem(
            "rpc-refresh:plug.X:agent-A",
            "not-a-number",
        );
        const { result } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-A", 5000),
        );
        expect(result.current.effectiveMs).toBe(5000);
    });

    it("scopes persistence by (pluginID, agentID)", () => {
        const { result: a } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-A", 0),
        );
        a.current.chooseInterval(15000);
        // Different plugin, same agent — should NOT see plug.X's
        // choice; default kicks in.
        const { result: b } = renderHook(() =>
            useRefreshInterval("plug.Y", "agent-A", 0),
        );
        expect(b.current.effectiveMs).toBe(0);
        // Same plugin, different agent — also independent.
        const { result: c } = renderHook(() =>
            useRefreshInterval("plug.X", "agent-B", 0),
        );
        expect(c.current.effectiveMs).toBe(0);
    });
});

describe("<MetaStrip>", () => {
    it("appends an orphan option when intervalMs isn't in the canonical list", () => {
        // 7000 is not in INTERVAL_OPTIONS; we want the <select> to
        // still display it rather than going blank.
        render(
            <MetaStrip
                dataUpdatedAt={Date.now()}
                isFetching={false}
                onRefresh={() => {}}
                intervalMs={7000}
                onIntervalChange={() => {}}
            />,
        );
        const select = screen.getByRole("combobox", {
            name: /auto refresh interval/i,
        }) as HTMLSelectElement;
        expect(select.value).toBe("7000");
        // Synthetic option has the seconds-style label.
        expect(
            Array.from(select.options).some((o) => o.label === "7s"),
        ).toBe(true);
    });

    it("disables the refresh button while fetching", () => {
        render(
            <MetaStrip
                dataUpdatedAt={0}
                isFetching={true}
                onRefresh={() => {}}
                intervalMs={0}
                onIntervalChange={() => {}}
            />,
        );
        const btn = screen.getByRole("button", { name: /refresh now/i });
        expect(btn).toBeDisabled();
    });

    it("emits onIntervalChange with the chosen number when the operator picks", () => {
        let picked: number | null = null;
        render(
            <MetaStrip
                dataUpdatedAt={Date.now()}
                isFetching={false}
                onRefresh={() => {}}
                intervalMs={0}
                onIntervalChange={(ms) => {
                    picked = ms;
                }}
            />,
        );
        const select = screen.getByRole("combobox", {
            name: /auto refresh interval/i,
        });
        fireEvent.change(select, { target: { value: "10000" } });
        expect(picked).toBe(10000);
    });
});
