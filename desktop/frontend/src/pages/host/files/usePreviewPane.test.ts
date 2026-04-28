import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { usePreviewPane } from "./usePreviewPane";

beforeEach(() => {
    window.localStorage.clear();
});

afterEach(() => {
    window.localStorage.clear();
});

describe("usePreviewPane", () => {
    it("starts closed when no persisted state exists", () => {
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(false);
    });

    it("toggle / close manage the open state", () => {
        const { result } = renderHook(() => usePreviewPane());

        act(() => result.current.toggle());
        expect(result.current.open).toBe(true);

        act(() => result.current.toggle());
        expect(result.current.open).toBe(false);

        // Close from already-closed is a no-op (no error).
        act(() => result.current.close());
        expect(result.current.open).toBe(false);

        act(() => result.current.setOpen(true));
        act(() => result.current.close());
        expect(result.current.open).toBe(false);
    });

    it("persists the open flag to localStorage", () => {
        const { result } = renderHook(() => usePreviewPane());

        act(() => result.current.setOpen(true));
        expect(window.localStorage.getItem("files.previewOpen")).toBe("true");

        act(() => result.current.setOpen(false));
        expect(window.localStorage.getItem("files.previewOpen")).toBe("false");
    });

    it("rehydrates the open flag across mounts", () => {
        // Pre-seed localStorage as if the previous session left the
        // pane open — the user expects to come back to the same
        // layout after reload.
        window.localStorage.setItem("files.previewOpen", "true");
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(true);
    });
});
