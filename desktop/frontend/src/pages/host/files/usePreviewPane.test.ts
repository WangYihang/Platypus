import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { usePreviewPane } from "./usePreviewPane";

// usePreviewPane now wraps `usePreference("ui.files.previewOpen")`,
// so the localStorage key is `platypus.pref.ui.files.previewOpen`.
// Default flipped to `true` — the auto-collapsing right rail in
// FileBrowser means "preview open by default" no longer wastes
// viewport on a placeholder when nothing is selected.

beforeEach(() => {
    window.localStorage.clear();
});

afterEach(() => {
    window.localStorage.clear();
});

const KEY = "platypus.pref.ui.files.previewOpen";

describe("usePreviewPane", () => {
    it("starts open when no persisted state exists", () => {
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(true);
    });

    it("toggle / close manage the open state", () => {
        const { result } = renderHook(() => usePreviewPane());

        // Default is open; one toggle closes.
        act(() => result.current.toggle());
        expect(result.current.open).toBe(false);

        act(() => result.current.toggle());
        expect(result.current.open).toBe(true);

        // Close from already-open is a transition.
        act(() => result.current.close());
        expect(result.current.open).toBe(false);

        // Close from already-closed is a no-op.
        act(() => result.current.close());
        expect(result.current.open).toBe(false);

        act(() => result.current.setOpen(true));
        act(() => result.current.close());
        expect(result.current.open).toBe(false);
    });

    it("persists the open flag to localStorage", () => {
        const { result } = renderHook(() => usePreviewPane());

        act(() => result.current.setOpen(false));
        expect(window.localStorage.getItem(KEY)).toBe(JSON.stringify(false));

        act(() => result.current.setOpen(true));
        expect(window.localStorage.getItem(KEY)).toBe(JSON.stringify(true));
    });

    it("rehydrates the open flag across mounts", () => {
        // Pre-seed localStorage as if the previous session left the
        // pane closed — the user expects to come back to the same
        // layout after reload.
        window.localStorage.setItem(KEY, JSON.stringify(false));
        const { result } = renderHook(() => usePreviewPane());
        expect(result.current.open).toBe(false);
    });
});
