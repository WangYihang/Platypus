// Streaming archive download.
//
// Replaces the legacy "fetch one 256 KiB chunk per HTTP request, build a
// tar in browser memory" path with a single fetch() to /fs/archive
// whose body is a real (compressed) archive built on the agent. Goals:
//
//   * end-to-end streaming so a multi-GB folder never sits in memory;
//   * progress reporting both via the response Content-Length /
//     X-Total-Bytes headers AND a per-chunk callback so progress bars
//     in the file browser update smoothly (the transfers tab gets the
//     same data via the WS event channel and renders independently);
//   * direct-to-disk save through the File System Access API where it
//     exists, falling back to a blob URL <a download> trigger;
//   * cancellation by AbortSignal so a "Cancel" button on the UI
//     tears down the in-flight fetch + closes the writable stream.
//
// Production callers wire onProgress to a UI hook OR rely on the WS
// FileTransferUpdated events; either is fine. The function returns
// the X-Transfer-Id header so the UI can correlate later WS events
// with this download (e.g. to flip a "downloading…" toast to "done").

import { authFetch } from "./auth";

export type ArchiveFormat = "tar" | "tar.gz" | "zip";

export interface DownloadArchiveOptions {
    projectId: string;
    agentId: string;
    paths: string[];
    format: ArchiveFormat;
    filename: string;
    followSymlinks?: boolean;
    compressionLevel?: number;
    /**
     * Called as bytes flow through. `total` is 0 when the server didn't
     * supply a length header (e.g. scan failed). `received` is
     * monotonically nondecreasing.
     */
    onProgress?: (received: number, total: number) => void;
    /**
     * AbortSignal — when fired aborts the underlying fetch and the
     * write-to-disk pipeline. The returned promise rejects with the
     * abort reason.
     */
    signal?: AbortSignal;
}

export interface DownloadArchiveResult {
    transferId: string;
    bytesWritten: number;
}

interface ShowSaveFilePickerOptions {
    suggestedName?: string;
    types?: Array<{
        description: string;
        accept: Record<string, string[]>;
    }>;
}

interface FileSystemFileHandleLike {
    createWritable(): Promise<FileSystemWritableFileStreamLike>;
}

interface FileSystemWritableFileStreamLike {
    write(chunk: BufferSource | Blob | string): Promise<void>;
    close(): Promise<void>;
    abort(reason?: unknown): Promise<void>;
}

type ShowSaveFilePicker = (
    opts?: ShowSaveFilePickerOptions,
) => Promise<FileSystemFileHandleLike>;

function getShowSaveFilePicker(): ShowSaveFilePicker | undefined {
    if (typeof window === "undefined") return undefined;
    const fn = (window as unknown as { showSaveFilePicker?: ShowSaveFilePicker })
        .showSaveFilePicker;
    return typeof fn === "function" ? fn : undefined;
}

function mimeForFormat(format: ArchiveFormat): string {
    switch (format) {
        case "tar":
            return "application/x-tar";
        case "tar.gz":
            return "application/gzip";
        case "zip":
            return "application/zip";
    }
}

function extForFormat(format: ArchiveFormat): string {
    switch (format) {
        case "tar":
            return ".tar";
        case "tar.gz":
            return ".tar.gz";
        case "zip":
            return ".zip";
    }
}

async function pickWritable(
    filename: string,
    format: ArchiveFormat,
): Promise<FileSystemWritableFileStreamLike | null> {
    const showSave = getShowSaveFilePicker();
    if (!showSave) return null;
    try {
        const handle = await showSave({
            suggestedName: filename,
            types: [
                {
                    description: format,
                    accept: { [mimeForFormat(format)]: [extForFormat(format)] },
                },
            ],
        });
        return await handle.createWritable();
    } catch (err) {
        // User dismissed the picker — surface as an abort so callers
        // can distinguish "save canceled" from "transfer failed".
        if ((err as { name?: string }).name === "AbortError") {
            throw err;
        }
        // Other errors (insecure context, permission policy) fall
        // back to the blob URL path.
        return null;
    }
}

function triggerBlobDownload(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.dataset.testArchiveDownload = "1";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1_000);
}

/**
 * downloadArchive POSTs to /fs/archive and streams the response body to
 * disk (or memory, then to disk via blob URL). Resolves on a clean EOF;
 * rejects on HTTP error, abort, or write failure.
 */
export async function downloadArchive(
    opts: DownloadArchiveOptions,
): Promise<DownloadArchiveResult> {
    const body: Record<string, unknown> = {
        paths: opts.paths,
        format: opts.format,
    };
    if (opts.followSymlinks) body.follow_symlinks = true;
    if (opts.compressionLevel) body.compression_level = opts.compressionLevel;

    const path = `/api/v1/projects/${encodeURIComponent(
        opts.projectId,
    )}/agents/${encodeURIComponent(opts.agentId)}/fs/archive`;

    const resp = await authFetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        signal: opts.signal,
    });

    if (!resp.ok) {
        const text = await resp.text().catch(() => "");
        throw new Error(text || `archive: HTTP ${resp.status}`);
    }
    const transferId = resp.headers.get("X-Transfer-Id") || "";
    const totalHeader = resp.headers.get("X-Total-Bytes");
    const contentLength = resp.headers.get("Content-Length");
    const total = Number(totalHeader || contentLength || 0) || 0;

    if (!resp.body) {
        // Some test runners return a Response without a stream body;
        // bail out cleanly so we don't deadlock.
        return { transferId, bytesWritten: 0 };
    }

    // Pick a sink: writable stream when available, otherwise an
    // in-memory accumulator we hand off to a blob URL trigger.
    const writable = await pickWritable(opts.filename, opts.format);
    const chunks: Uint8Array[] = [];

    let received = 0;
    const reader = resp.body.getReader();
    // Watch the abort signal explicitly so the read loop unwinds
    // promptly even when the underlying ReadableStream doesn't
    // forward abort cleanly (some Response implementations / test
    // mocks don't). Cancelling the reader unblocks any pending
    // read() with a rejected promise.
    let abortReason: unknown;
    const onAbort = () => {
        abortReason = opts.signal?.reason ?? new DOMException("aborted", "AbortError");
        reader.cancel(abortReason).catch(() => {});
    };
    if (opts.signal) {
        if (opts.signal.aborted) {
            onAbort();
        } else {
            opts.signal.addEventListener("abort", onAbort, { once: true });
        }
    }
    try {
        while (true) {
            if (opts.signal?.aborted) {
                throw abortReason ?? new DOMException("aborted", "AbortError");
            }
            const { done, value } = await reader.read();
            if (done) break;
            if (value && value.byteLength > 0) {
                received += value.byteLength;
                if (writable) {
                    await writable.write(value);
                } else {
                    chunks.push(value);
                }
                opts.onProgress?.(received, Math.max(total, received));
            }
        }
        // Final progress tick so the UI sees 100 % even when no
        // chunk crossed the threshold.
        opts.onProgress?.(received, Math.max(total, received));
    } catch (err) {
        if (writable) await writable.abort(err).catch(() => {});
        throw err;
    } finally {
        opts.signal?.removeEventListener("abort", onAbort);
    }

    if (writable) {
        await writable.close();
    } else {
        const blob = new Blob(chunks as unknown as BlobPart[], {
            type: mimeForFormat(opts.format),
        });
        triggerBlobDownload(blob, opts.filename);
    }
    return { transferId, bytesWritten: received };
}
