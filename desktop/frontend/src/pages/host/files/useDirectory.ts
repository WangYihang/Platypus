import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

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

    const queryKey = ["directory", projectID, sessionHash, path] as const;

    const { data, isFetching: loading, error, refetch } = useQuery({
        queryKey,
        queryFn: () => ListDir(projectID, sessionHash, path, 0, 0),
        // The directory contents are scoped to a live session — once
        // we leave, the cache value goes stale instantly. Keep the
        // entry around briefly so a quick "Up → re-cd same dir"
        // round-trip feels instant.
        staleTime: 5_000,
        // Persist the last-visited path on every successful fetch.
        // Putting this in the queryFn keeps the side effect tied to
        // an actual successful directory load.
    });

    useEffect(() => {
        if (data) writeSavedPath(sessionHash, path);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [data, sessionHash, path]);

    const cd = useCallback((targetPath: string) => {
        setPath(targetPath);
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
