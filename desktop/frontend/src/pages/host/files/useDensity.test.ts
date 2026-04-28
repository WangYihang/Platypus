import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useDensity } from "./useDensity";

beforeEach(() => {
    window.localStorage.clear();
});

afterEach(() => {
    window.localStorage.clear();
});

describe("useDensity", () => {
    it("defaults to 'compact' when no value is persisted", () => {
        // Compact is the default because file lists are typically long
        // and the operator wants as many rows on screen as possible
        // before scrolling.
        const { result } = renderHook(() => useDensity());
        expect(result.current[0]).toBe("compact");
    });

    it("persists the chosen density and rehydrates across mounts", () => {
        const { result, unmount } = renderHook(() => useDensity());
        act(() => result.current[1]("comfortable"));
        expect(result.current[0]).toBe("comfortable");
        expect(window.localStorage.getItem("platypus:filesDensity")).toBe(
            "comfortable",
        );

        unmount();

        const remount = renderHook(() => useDensity());
        expect(remount.result.current[0]).toBe("comfortable");
    });

    it("falls back to the default when the persisted value is garbage", () => {
        // A migration / typo shouldn't lock the user out of toggling —
        // unknown values silently revert to the default on next load.
        window.localStorage.setItem("platypus:filesDensity", "ultradense");
        const { result } = renderHook(() => useDensity());
        expect(result.current[0]).toBe("compact");
    });
});
