import { useCallback, useState } from "react";

// useViewMode persists the file browser's list/grid preference across
// sessions. The key is global (not per-session) so the user picks once
// and the choice follows them across hosts. Garbage / older values
// fall back to "list" — the safe default that has always worked.

export type ViewMode = "list" | "grid";

const KEY = "platypus:filesViewMode";

function read(): ViewMode {
    try {
        const v = localStorage.getItem(KEY);
        if (v === "list" || v === "grid") return v;
    } catch {
        // localStorage may be unavailable in some embedded WebViews.
    }
    return "list";
}

export function useViewMode(): [ViewMode, (next: ViewMode) => void] {
    const [mode, setMode] = useState<ViewMode>(() => read());
    const set = useCallback((next: ViewMode) => {
        setMode(next);
        try {
            localStorage.setItem(KEY, next);
        } catch {
            // ignore
        }
    }, []);
    return [mode, set];
}
