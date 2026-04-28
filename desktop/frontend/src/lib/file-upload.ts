// file-upload.ts — streaming-upload primitive shared by the file
// browser drag-drop zone and the future "upload" button. PUTs once
// to /fs/upload with the File as the request body, returns the
// X-Transfer-Id so the UI can correlate later WS progress events
// (the server pushes FileTransferUpdated events as the body flows;
// the lib also reports a final local 100% tick so the browser-side
// store updates even when WS is offline).
//
// Why a thin wrapper rather than firing fetch directly from the UI:
//   * one place to URL-encode the path (tests cover this);
//   * one place to cope with non-2xx responses uniformly;
//   * keeps the signal / progress contract symmetric with
//     archive-download.ts so the file browser can fire either
//     direction with the same ergonomics.

import { authFetch } from "./auth";

export interface UploadFileOptions {
    projectId: string;
    agentId: string;
    /**
     * Directory on the agent's filesystem the file lands in. The
     * file's name is appended to form the final path; the lib
     * handles separator normalisation so callers can pass "/var/log"
     * or "/var/log/" interchangeably.
     */
    remoteDir: string;
    file: File;
    /** Append to existing file rather than truncate. Default false. */
    append?: boolean;
    /** Create parent directories if missing. Default true — matches
     * the desktop drag-drop UX where dropping into a fresh tree
     * shouldn't bail on a missing intermediate dir. */
    mkdirs?: boolean;
    /**
     * Called as bytes are sent. The browser doesn't expose request
     * upload progress for fetch() reliably, so this lib only fires
     * a final 100% tick after the response arrives. Live progress
     * comes from the WS FileTransferUpdated events the server emits
     * while the body flows.
     */
    onProgress?: (received: number, total: number) => void;
    /** AbortSignal — fires the underlying fetch's abort, the server
     * cancels the in-flight transfer via Body.Close(). */
    signal?: AbortSignal;
}

export interface UploadFileResult {
    transferId: string;
    bytesWritten: number;
}

function joinPath(dir: string, name: string): string {
    const trimmed = dir.replace(/\/+$/, "");
    return trimmed === "" ? `/${name}` : `${trimmed}/${name}`;
}

/**
 * uploadFile streams `file` to the named directory on the agent.
 * Resolves with the server's bytes_written + transfer_id; rejects
 * with the response body text on any non-2xx.
 */
export async function uploadFile(opts: UploadFileOptions): Promise<UploadFileResult> {
    const remotePath = joinPath(opts.remoteDir, opts.file.name);
    // Build the query manually with encodeURIComponent so spaces in
    // remote paths land as %20 rather than the URLSearchParams "+"
    // — paths with literal "+" characters then round-trip cleanly
    // (the server's c.Query unescapes both %20 and "+", but our
    // tests assert on %20 because the rest of the codebase encodes
    // paths the same way).
    const qs =
        `path=${encodeURIComponent(remotePath)}` +
        `&total_bytes=${opts.file.size}` +
        `&mkdirs=${opts.mkdirs === false ? "false" : "true"}` +
        (opts.append ? `&append=true` : "");
    const path = `/api/v1/projects/${encodeURIComponent(
        opts.projectId,
    )}/agents/${encodeURIComponent(opts.agentId)}/fs/upload?${qs}`;

    const resp = await authFetch(path, {
        method: "PUT",
        headers: { "Content-Type": "application/octet-stream" },
        body: opts.file,
        signal: opts.signal,
    });

    if (!resp.ok) {
        const text = await resp.text().catch(() => "");
        throw new Error(text || `upload: HTTP ${resp.status}`);
    }
    let body: { bytes_written?: number; transfer_id?: string } = {};
    try {
        body = await resp.json();
    } catch {
        // Tolerate empty bodies — fall back to header-derived metadata.
    }
    const transferId = body.transfer_id || resp.headers.get("X-Transfer-Id") || "";
    const bytesWritten = body.bytes_written ?? opts.file.size;
    opts.onProgress?.(bytesWritten, opts.file.size);
    return { transferId, bytesWritten };
}
