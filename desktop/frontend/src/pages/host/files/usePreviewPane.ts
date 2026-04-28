import { useCallback, useEffect, useState } from "react";

// usePreviewPane is the storage + keyboard layer behind the Quick-Look
// style preview pane. It is *not* concerned with what lives inside the
// pane (that's the viewer dispatch in FileBrowser); it only tracks
// open/closed state, the persisted width, and the keys that toggle
// them. Splitting it out keeps FileBrowser leaner and lets the tests
// drive the state machine without mounting the whole browser.
//
// Persistence is intentionally per-browser (localStorage), not
// per-server: the user's preferred layout follows them across project
// switches but doesn't roam to other devices.

const KEY_OPEN = "files.previewOpen";
const KEY_WIDTH = "files.previewWidth";

export const PREVIEW_DEFAULT_WIDTH = 420;
export const PREVIEW_MIN_WIDTH = 240;
export const PREVIEW_MAX_WIDTH = 1200;

function loadOpen(): boolean {
    if (typeof window === "undefined") return false;
    try {
        return window.localStorage.getItem(KEY_OPEN) === "true";
    } catch {
        return false;
    }
}

function loadWidth(): number {
    if (typeof window === "undefined") return PREVIEW_DEFAULT_WIDTH;
    try {
        const raw = window.localStorage.getItem(KEY_WIDTH);
        if (!raw) return PREVIEW_DEFAULT_WIDTH;
        const n = Number.parseInt(raw, 10);
        if (!Number.isFinite(n)) return PREVIEW_DEFAULT_WIDTH;
        return clampWidth(n);
    } catch {
        return PREVIEW_DEFAULT_WIDTH;
    }
}

export function clampWidth(n: number): number {
    if (n < PREVIEW_MIN_WIDTH) return PREVIEW_MIN_WIDTH;
    if (n > PREVIEW_MAX_WIDTH) return PREVIEW_MAX_WIDTH;
    return n;
}

export interface PreviewPane {
    open: boolean;
    width: number;
    setOpen: (open: boolean) => void;
    toggle: () => void;
    close: () => void;
    setWidth: (next: number) => void;
}

export function usePreviewPane(): PreviewPane {
    const [open, setOpenRaw] = useState<boolean>(loadOpen);
    const [width, setWidthRaw] = useState<number>(loadWidth);

    useEffect(() => {
        try {
            window.localStorage.setItem(KEY_OPEN, String(open));
        } catch {
            // Quota errors etc. are best-effort; preview state is
            // not critical so we silently lose persistence.
        }
    }, [open]);

    useEffect(() => {
        try {
            window.localStorage.setItem(KEY_WIDTH, String(width));
        } catch {
            // see above
        }
    }, [width]);

    const setWidth = useCallback((next: number) => {
        setWidthRaw(clampWidth(next));
    }, []);

    const setOpen = useCallback((next: boolean) => {
        setOpenRaw(next);
    }, []);

    const toggle = useCallback(() => setOpenRaw((o) => !o), []);
    const close = useCallback(() => setOpenRaw(false), []);

    return { open, width, setOpen, setWidth, toggle, close };
}
