import { useCallback, useEffect, useState } from "react";

import { ListDir } from "../../../../wailsjs/go/app/App";
import type { FileEntryDTO } from "../../../platform/App.web";

// useDirectory manages the current directory listing for a session.
// Remembers the last visited path in localStorage so reopening the tab
// lands the user where they were.

const LAST_DIR_KEY = (sessionHash: string) => `platypus:lastDir:${sessionHash}`;

export interface DirectoryState {
    path: string;
    entries: FileEntryDTO[];
    total: number;
    eof: boolean;
    loading: boolean;
    error: string | null;
}

export function useDirectory(sessionHash: string, initialPath = "/") {
    const [state, setState] = useState<DirectoryState>(() => {
        let path = initialPath;
        try {
            const saved = localStorage.getItem(LAST_DIR_KEY(sessionHash));
            if (saved) path = saved;
        } catch {
            // localStorage may be unavailable in some embedded WebViews.
        }
        return { path, entries: [], total: 0, eof: true, loading: false, error: null };
    });

    const load = useCallback(
        async (targetPath: string) => {
            setState((s) => ({ ...s, path: targetPath, loading: true, error: null }));
            try {
                const result = await ListDir(sessionHash, targetPath, 0, 0);
                setState({
                    path: targetPath,
                    entries: result.entries,
                    total: result.total,
                    eof: result.eof,
                    loading: false,
                    error: null,
                });
                try {
                    localStorage.setItem(LAST_DIR_KEY(sessionHash), targetPath);
                } catch {
                    // ignore
                }
            } catch (err) {
                setState((s) => ({
                    ...s,
                    loading: false,
                    error: String(err instanceof Error ? err.message : err),
                }));
            }
        },
        [sessionHash],
    );

    const cd = useCallback(
        (targetPath: string) => {
            load(targetPath);
        },
        [load],
    );

    const reload = useCallback(() => load(state.path), [load, state.path]);

    useEffect(() => {
        load(state.path);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [sessionHash]);

    return { ...state, cd, reload };
}
