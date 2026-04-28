import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useViewMode } from "./useViewMode";

// useViewMode now wraps `usePreference("ui.files.viewMode")`, so
// the localStorage key is `platypus.pref.ui.files.viewMode`. Hook
// contract is unchanged: `[ViewMode, setter]`, default "list",
// garbage in → default out.

beforeEach(() => {
    localStorage.clear();
});

afterEach(() => {
    localStorage.clear();
});

const KEY = "platypus.pref.ui.files.viewMode";

describe("useViewMode", () => {
    it("defaults to list when nothing is persisted", () => {
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("list");
    });

    it("restores the persisted mode on mount", () => {
        localStorage.setItem(KEY, JSON.stringify("grid"));
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("grid");
    });

    it("persists subsequent changes to localStorage", () => {
        const { result } = renderHook(() => useViewMode());
        act(() => result.current[1]("grid"));
        expect(result.current[0]).toBe("grid");
        expect(localStorage.getItem(KEY)).toBe(JSON.stringify("grid"));
        act(() => result.current[1]("list"));
        expect(localStorage.getItem(KEY)).toBe(JSON.stringify("list"));
    });

    it("ignores garbage stored values and falls back to list", () => {
        localStorage.setItem(KEY, "{not json");
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("list");
    });
});
