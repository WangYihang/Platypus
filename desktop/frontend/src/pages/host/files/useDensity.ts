import { useCallback, useState } from "react";

// useDensity persists the FileTable row-height preference (compact /
// comfortable) so a user who tightens the layout once doesn't have
// to re-toggle on every reload. Stored alongside ViewMode under a
// global (not per-host) key so the choice follows them.

export type Density = "compact" | "comfortable";

const KEY = "platypus:filesDensity";

function read(): Density {
    try {
        const v = localStorage.getItem(KEY);
        if (v === "compact" || v === "comfortable") return v;
    } catch {
        // localStorage may be unavailable in some embedded WebViews.
    }
    return "comfortable";
}

export function useDensity(): [Density, (next: Density) => void] {
    const [density, setDensity] = useState<Density>(() => read());
    const set = useCallback((next: Density) => {
        setDensity(next);
        try {
            localStorage.setItem(KEY, next);
        } catch {
            // ignore
        }
    }, []);
    return [density, set];
}
