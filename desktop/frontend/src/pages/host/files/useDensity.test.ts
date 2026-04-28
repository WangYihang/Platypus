import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useDensity } from "./useDensity";

// useDensity now wraps `usePreference("ui.density")` (see
// lib/preferences.ts), so localStorage assertions are against the
// `platypus.pref.ui.density` key. Public hook contract is unchanged:
// `[Density, setter]` tuple, default "compact", garbage in →
// default out.

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
        expect(window.localStorage.getItem("platypus.pref.ui.density")).toBe(
            JSON.stringify("comfortable"),
        );

        unmount();

        const remount = renderHook(() => useDensity());
        expect(remount.result.current[0]).toBe("comfortable");
    });

    it("falls back to the default when the persisted value is garbage", () => {
        // A migration / typo shouldn't lock the user out of toggling —
        // unknown values silently revert to the default on next load.
        window.localStorage.setItem("platypus.pref.ui.density", "{not json");
        const { result } = renderHook(() => useDensity());
        expect(result.current[0]).toBe("compact");
    });
});
