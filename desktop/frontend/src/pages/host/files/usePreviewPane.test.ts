import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
    PREVIEW_DEFAULT_WIDTH,
    PREVIEW_MAX_WIDTH,
    PREVIEW_MIN_WIDTH,
    clampWidth,
    usePreviewPane,
} from "./usePreviewPane";

beforeEach(() => {
    window.localStorage.clear();
});

afterEach(() => {
    window.localStorage.clear();
});

describe("usePreviewPane", () => {
    it("starts closed at the default width when no persisted state exists", () => {
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(false);
        expect(result.current.width).toBe(PREVIEW_DEFAULT_WIDTH);
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

    it("persists the width and rehydrates it across mounts", () => {
        const { result, unmount } = renderHook(() => usePreviewPane());

        act(() => result.current.setWidth(500));
        expect(result.current.width).toBe(500);
        expect(window.localStorage.getItem("files.previewWidth")).toBe("500");

        unmount();

        const remount = renderHook(() => usePreviewPane());
        expect(remount.result.current.width).toBe(500);
    });

    it("rehydrates the open flag across mounts", () => {
        // Pre-seed localStorage as if the previous session left the
        // pane open — the user expects to come back to the same
        // layout after reload.
        window.localStorage.setItem("files.previewOpen", "true");
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(true);
    });

    it("clamps width to the [MIN, MAX] range on set and on rehydrate", () => {
        const { result } = renderHook(() => usePreviewPane());

        act(() => result.current.setWidth(50));
        expect(result.current.width).toBe(PREVIEW_MIN_WIDTH);

        act(() => result.current.setWidth(99999));
        expect(result.current.width).toBe(PREVIEW_MAX_WIDTH);

        // Garbage in localStorage falls back to the default.
        window.localStorage.setItem("files.previewWidth", "not-a-number");
        const remount = renderHook(() => usePreviewPane());
        expect(remount.result.current.width).toBe(PREVIEW_DEFAULT_WIDTH);
    });

    it("clampWidth pure helper covers the bounds explicitly", () => {
        // Pinned in its own assertion so a regression in the bounds
        // can't be lost to a hook test that exercises both at once.
        expect(clampWidth(100)).toBe(PREVIEW_MIN_WIDTH);
        expect(clampWidth(99999)).toBe(PREVIEW_MAX_WIDTH);
        expect(clampWidth(420)).toBe(420);
    });
});
