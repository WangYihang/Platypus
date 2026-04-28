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
    it("defaults to 'comfortable' when no value is persisted", () => {
        const { result } = renderHook(() => useDensity());
        expect(result.current[0]).toBe("comfortable");
    });

    it("persists the chosen density and rehydrates across mounts", () => {
        const { result, unmount } = renderHook(() => useDensity());
        act(() => result.current[1]("compact"));
        expect(result.current[0]).toBe("compact");
        expect(window.localStorage.getItem("platypus:filesDensity")).toBe("compact");

        unmount();

        const remount = renderHook(() => useDensity());
        expect(remount.result.current[0]).toBe("compact");
    });

    it("falls back to the default when the persisted value is garbage", () => {
        // A migration / typo shouldn't lock the user out of toggling —
        // unknown values silently revert to comfortable on next load.
        window.localStorage.setItem("platypus:filesDensity", "ultradense");
        const { result } = renderHook(() => useDensity());
        expect(result.current[0]).toBe("comfortable");
    });
});
