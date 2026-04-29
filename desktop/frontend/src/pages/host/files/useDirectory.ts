import { useCallback, useEffect, useRef, useState } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";

import { ListDir } from "@wails/go/app/App";
import type { FileEntryDTO } from "@wails/go/app/App";

// useDirectory wraps the file-listing RPC for a session. Remembers
// the last visited path in localStorage so reopening the tab lands
// the user where they were. Now backed by react-query so the
// listing is cached per `(sessionHash, path)` — bouncing between
// directories and back is instant after the first visit, with a
// background refetch behind it.
//
// Why a custom localStorage key instead of `lib/preferences`? The
// path is per-session-hash; the typed registry's keys are global,
// and adding a Map<sessionHash, path> there would just move the
// complexity. Sticking with the existing key keeps the diff
// surgical.

const LAST_DIR_KEY = (sessionHash: string) => `platypus:lastDir:${sessionHash}`;

export interface DirectoryState {
    path: string;
    entries: FileEntryDTO[];
    total: number;
    eof: boolean;
    loading: boolean;
    error: string | null;
}

function readSavedPath(sessionHash: string, fallback: string): string {
    try {
        const saved = localStorage.getItem(LAST_DIR_KEY(sessionHash));
        if (saved) return saved;
    } catch {
        // localStorage may be unavailable in some embedded WebViews.
    }
    return fallback;
}

function writeSavedPath(sessionHash: string, path: string) {
    try {
        localStorage.setItem(LAST_DIR_KEY(sessionHash), path);
    } catch {
        // ignore
    }
}

export function useDirectory(projectID: string, sessionHash: string, initialPath = "/") {
    const queryClient = useQueryClient();
    const [path, setPath] = useState<string>(() =>
        readSavedPath(sessionHash, initialPath),
    );
    // Per-session navigation history: a back stack of paths the user
    // has visited and a forward stack populated when they go back.
    // Modelled on a browser tab — clicking on a directory truncates
    // the forward stack, the toolbar's < / > buttons pop between
    // them. Reset when the session changes (see effect below) so the
    // history doesn't bleed between hosts.
    const [history, setHistory] = useState<{ back: string[]; forward: string[] }>({
        back: [],
        forward: [],
    });

    const queryKey = ["directory", projectID, sessionHash, path] as const;

    const { data, isFetching: loading, error, refetch } = useQuery({
        queryKey,
        queryFn: () => ListDir(projectID, sessionHash, path, 0, 0),
        // The directory contents are scoped to a live session — once
        // we leave, the cache value goes stale instantly. Keep the
        // entry around briefly so a quick "Up → re-cd same dir"
        // round-trip feels instant.
        staleTime: 5_000,
        // `placeholderData: keepPreviousData` keeps the previous
        // directory's entries rendered while the new path's fetch is
        // in flight. Without this the grid blanks for the duration of
        // every cd round-trip — bad UX, and the original
        // useState-driven implementation (commit 92f9c94) preserved
        // the entries explicitly via `setState({...s, loading:true})`.
        placeholderData: keepPreviousData,
    });

    useEffect(() => {
        if (data) writeSavedPath(sessionHash, path);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [data, sessionHash, path]);

    // Mirror `path` into a ref so back() / forward() can compute
    // `currentPathRef.current` without re-creating the closures on
    // every state change. Without the ref the back stack would
    // capture a stale path snapshot and push the *previous* path
    // onto forward instead of the current one.
    const currentPathRef = useRef(path);
    useEffect(() => {
        currentPathRef.current = path;
    }, [path]);

    const cd = useCallback((targetPath: string) => {
        setPath((prev) => {
            if (prev === targetPath) return prev;
            // Forward-truncate: a fresh `cd` always starts a new
            // branch in the history tree, just like a browser
            // navigation invalidates the forward stack.
            setHistory((h) => ({ back: [...h.back, prev], forward: [] }));
            return targetPath;
        });
    }, []);

    const back = useCallback(() => {
        setHistory((h) => {
            if (h.back.length === 0) return h;
            const target = h.back[h.back.length - 1];
            setPath((prev) => {
                if (prev === target) return prev;
                return target;
            });
            // Push the *current* path onto the forward stack so the
            // forward arrow returns the user to where they were.
            return {
                back: h.back.slice(0, -1),
                forward: [...h.forward, currentPathRef.current],
            };
        });
    }, []);

    const forward = useCallback(() => {
        setHistory((h) => {
            if (h.forward.length === 0) return h;
            const target = h.forward[h.forward.length - 1];
            setPath((prev) => {
                if (prev === target) return prev;
                return target;
            });
            return {
                back: [...h.back, currentPathRef.current],
                forward: h.forward.slice(0, -1),
            };
        });
    }, []);

    const reload = useCallback(() => {
        // useQuery's refetch fires the queryFn for the current key
        // and updates `data`; that's exactly the "refresh this
        // listing without changing path" semantics callers want.
        void refetch();
    }, [refetch]);

    // Reset the path when the session changes — switching hosts
    // shouldn't carry a stale `/var/log` over from the previous one.
    useEffect(() => {
        setPath(readSavedPath(sessionHash, initialPath));
        // History is per-session: switching hosts should give the
        // user a fresh navigation slate, not a back stack pointing
        // into a directory tree that doesn't exist on the new host.
        setHistory({ back: [], forward: [] });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [sessionHash]);

    // Provide a typed errorString for backwards-compatible call sites
    // that render `error` as a string.
    const errorString =
        error == null
            ? null
            : String(error instanceof Error ? error.message : error);

    return {
        path,
        entries: data?.entries ?? [],
        total: data?.total ?? 0,
        eof: data?.eof ?? true,
        loading,
        error: errorString,
        cd,
        back,
        forward,
        canBack: history.back.length > 0,
        canForward: history.forward.length > 0,
        reload,
        // Expose the queryClient so consumers (e.g. mutations like
        // delete / rename / mkdir) can `invalidateQueries({ queryKey:
        // ["directory", ...] })` without re-deriving the key.
        invalidate: () =>
            queryClient.invalidateQueries({
                queryKey: ["directory", projectID, sessionHash, path],
            }),
    };
}
