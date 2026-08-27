import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { ReadFile } from "@wails/go/app/App";
import { fsReadPreviewURL } from "@/lib/fs-preview";

// Shared plumbing for "read a file off the agent and show it".
//
// Six viewers each carried their own copy of bytesFromWailsRead and two
// carried their own decodeText, and every one of them hand-rolled the
// same load: a cancelled flag, setLoading/setError before the await,
// a try/catch that has to re-check cancelled in both arms, and a
// cleanup that flips the flag. That is what useQuery does, and this
// project already uses it for every other kind of fetch — these
// predate that, or were written without noticing.
//
// The copies had already started to disagree: Thumbnail's
// bytesFromWailsRead returned null where the other five threw, so a
// malformed read was a silent blank tile there and a visible error
// everywhere else.

// bytesFromWailsRead normalises what the Wails binding hands back.
// The Go side returns []byte, which arrives as a Uint8Array under the
// desktop runtime and as a plain number[] over the web JSON bridge.
export function bytesFromWailsRead(raw: unknown): Uint8Array {
    if (raw instanceof Uint8Array) return raw;
    if (Array.isArray(raw)) return new Uint8Array(raw as number[]);
    throw new Error(`unexpected ReadFile shape: ${typeof raw}`);
}

// decodeText reads bytes as UTF-8, falling back to Latin-1 for files
// with invalid UTF-8 sequences so a script with a stray byte still
// opens instead of erroring.
export function decodeText(bytes: Uint8Array): string {
    try {
        return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch {
        return new TextDecoder("latin1").decode(bytes);
    }
}

export interface RemoteFileArgs {
    projectID: string;
    sessionHash: string;
    path: string;
    /** Skip the read entirely — e.g. the file is over an inline size cap. */
    enabled?: boolean;
}

export function remoteFileKey(a: RemoteFileArgs) {
    return ["remoteFile", a.projectID, a.sessionHash, a.path] as const;
}

/** Whole-file read, normalised to bytes. */
export function useRemoteFileBytes(args: RemoteFileArgs) {
    const { projectID, sessionHash, path, enabled = true } = args;
    return useQuery({
        queryKey: remoteFileKey({ projectID, sessionHash, path }),
        queryFn: async () => bytesFromWailsRead(await ReadFile(projectID, sessionHash, path, 0, 0)),
        enabled,
        // File contents are fetched to be displayed once. Re-reading on
        // window focus would re-download the whole thing behind a
        // viewer the operator is already looking at.
        refetchOnWindowFocus: false,
        retry: false,
    });
}

/** Whole-file read, decoded as text. */
export function useRemoteFileText(args: RemoteFileArgs) {
    const q = useRemoteFileBytes(args);
    const text = useMemo(() => (q.data ? decodeText(q.data) : null), [q.data]);
    return { ...q, text };
}

export interface RemoteObjectURLArgs extends RemoteFileArgs {
    /** Blob type. Falls back to the browser's sniffing when empty. */
    mime?: string;
}

/**
 * Whole-file read exposed as an object URL, revoked when it changes or
 * the caller unmounts.
 *
 * The URL is derived rather than pushed into state, so nothing here
 * writes state from an effect; the effect exists only to own the
 * revoke, which is the one genuinely external thing in the chain.
 */
export function useRemoteObjectURL(args: RemoteObjectURLArgs) {
    const { mime, ...fileArgs } = args;
    const q = useRemoteFileBytes(fileArgs);

    const url = useMemo(() => {
        if (!q.data) return null;
        return URL.createObjectURL(new Blob([q.data as BlobPart], { type: mime || undefined }));
    }, [q.data, mime]);

    useEffect(() => {
        if (!url) return;
        return () => URL.revokeObjectURL(url);
    }, [url]);

    return { ...q, url };
}

/**
 * A URL the browser can load the file from, by whichever route this
 * build has.
 *
 * Web mode mints a short-lived signed preview URL and lets the browser
 * stream it — no bytes through React. Desktop reads via the Wails
 * binding and wraps the result in a blob URL. MediaViewer and
 * PdfViewer each carried their own copy of that branch plus its own
 * loading/error/cancel scaffolding; the choice is a property of "get
 * me a URL for this remote file", not of either viewer.
 *
 * Only the blob URL is ours to revoke — a signed URL is a plain string
 * and revoking it would be meaningless.
 */
export function useRemotePreviewURL(args: RemoteObjectURLArgs) {
    const { projectID, sessionHash, path, mime, enabled = true } = args;
    const streamed = import.meta.env.MODE === "web";

    const q = useQuery({
        queryKey: [...remoteFileKey({ projectID, sessionHash, path }), streamed ? "url" : "bytes"],
        queryFn: async (): Promise<string | Uint8Array> =>
            streamed
                ? await fsReadPreviewURL(projectID, sessionHash, path)
                : bytesFromWailsRead(await ReadFile(projectID, sessionHash, path, 0, 0)),
        enabled,
        refetchOnWindowFocus: false,
        retry: false,
    });

    const url = useMemo(() => {
        if (!q.data) return null;
        if (typeof q.data === "string") return q.data;
        return URL.createObjectURL(new Blob([q.data as BlobPart], { type: mime || undefined }));
    }, [q.data, mime]);

    const ownsURL = url !== null && typeof q.data !== "string";
    useEffect(() => {
        if (!url || !ownsURL) return;
        return () => URL.revokeObjectURL(url);
    }, [url, ownsURL]);

    return { ...q, url };
}

export interface RemoteFileRangeArgs extends RemoteFileArgs {
    /** Byte offset to start at. */
    offset: number;
    /** How many bytes to ask for. */
    length: number;
}

/**
 * A byte range of a remote file, decoded as text.
 *
 * Keyed on the range as well as the path, so paging through a large
 * file walks the cache rather than refetching a page the operator
 * already visited — and, more to the point, so the offset drives the
 * read. The paged viewer used to have that backwards: a loadPage()
 * callback fetched and then set the offset it had just used, which
 * made the offset a result of the fetch instead of its input.
 */
export function useRemoteFileRangeText(args: RemoteFileRangeArgs) {
    const { projectID, sessionHash, path, offset, length, enabled = true } = args;
    const q = useQuery({
        queryKey: [...remoteFileKey({ projectID, sessionHash, path }), "range", offset, length],
        queryFn: async () =>
            bytesFromWailsRead(await ReadFile(projectID, sessionHash, path, offset, length)),
        enabled,
        refetchOnWindowFocus: false,
        retry: false,
        // Keep the previous page on screen while the next one loads,
        // instead of flashing empty between pages.
        placeholderData: (prev) => prev,
    });
    const text = useMemo(() => (q.data ? decodeText(q.data) : null), [q.data]);
    return { ...q, text };
}
