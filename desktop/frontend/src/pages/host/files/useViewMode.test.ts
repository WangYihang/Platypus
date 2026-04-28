import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useViewMode } from "./useViewMode";

beforeEach(() => {
    localStorage.clear();
});

afterEach(() => {
    localStorage.clear();
});

describe("useViewMode", () => {
    it("defaults to list when nothing is persisted", () => {
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("list");
    });

    it("restores the persisted mode on mount", () => {
        localStorage.setItem("platypus:filesViewMode", "grid");
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("grid");
    });

    it("persists subsequent changes to localStorage", () => {
        const { result } = renderHook(() => useViewMode());
        act(() => result.current[1]("grid"));
        expect(result.current[0]).toBe("grid");
        expect(localStorage.getItem("platypus:filesViewMode")).toBe("grid");
        act(() => result.current[1]("list"));
        expect(localStorage.getItem("platypus:filesViewMode")).toBe("list");
    });

    it("ignores garbage stored values and falls back to list", () => {
        localStorage.setItem("platypus:filesViewMode", "tree");
        const { result } = renderHook(() => useViewMode());
        expect(result.current[0]).toBe("list");
    });
});
