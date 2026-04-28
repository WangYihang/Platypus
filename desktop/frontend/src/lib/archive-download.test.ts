import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

// archive-download.ts is the streaming-download primitive shared by
// the file browser (folder downloads) and the transfers tab (retry).
// It POSTs to /fs/archive, drains the streaming response, and either:
//   * writes through a File System Access API FileSystemWritableFileStream
//     when window.showSaveFilePicker is available, OR
//   * accumulates chunks in-memory and triggers an <a download> blob URL.
//
// Progress reaches the UI via the onProgress callback (called as bytes
// flow through) AND via the optional WS event subscription separately
// (not exercised here).

vi.mock("./auth", () => ({
    authFetch: vi.fn(),
    getSession: () => ({ serverURL: "https://example.test", sessionToken: "tok" }),
}));

import { authFetch } from "./auth";
import { downloadArchive } from "./archive-download";

const authFetchMock = vi.mocked(authFetch);

beforeEach(() => {
    authFetchMock.mockReset();
});

// streamFromChunks builds a fake fetch Response whose body is a
// ReadableStream that emits the supplied chunks in order.
function streamFromChunks(
    chunks: Uint8Array[],
    headers: Record<string, string> = {},
): Response {
    const stream = new ReadableStream<Uint8Array>({
        async start(controller) {
            for (const ch of chunks) {
                controller.enqueue(ch);
            }
            controller.close();
        },
    });
    const headersObj = new Headers({
        "Content-Type": "application/gzip",
        ...headers,
    });
    return new Response(stream, { status: 200, headers: headersObj });
}

afterEach(() => {
    // Strip any lingering anchors created by the blob-URL fallback so
    // successive tests don't accumulate them in the DOM.
    document
        .querySelectorAll("a[data-test-archive-download]")
        .forEach((el) => el.remove());
});

describe("downloadArchive", () => {
    it("POSTs paths + format to the project-scoped /fs/archive endpoint", async () => {
        authFetchMock.mockResolvedValueOnce(
            streamFromChunks([new Uint8Array([1, 2, 3])], {
                "X-Transfer-Id": "ft-1",
                "X-Total-Bytes": "3",
            }),
        );
        await downloadArchive({
            projectId: "p1",
            agentId: "a1",
            paths: ["/etc/hosts"],
            format: "tar.gz",
            filename: "etc.tar.gz",
        });
        expect(authFetchMock).toHaveBeenCalledTimes(1);
        const [path, init] = authFetchMock.mock.calls[0];
        expect(path).toBe("/api/v1/projects/p1/agents/a1/fs/archive");
        expect(init?.method).toBe("POST");
        const body = JSON.parse(init?.body as string);
        expect(body).toEqual({ paths: ["/etc/hosts"], format: "tar.gz" });
    });

    it("passes through follow_symlinks and compression_level when provided", async () => {
        authFetchMock.mockResolvedValueOnce(
            streamFromChunks([new Uint8Array([0])], {}),
        );
        await downloadArchive({
            projectId: "p1",
            agentId: "a1",
            paths: ["/x"],
            format: "tar",
            filename: "x.tar",
            followSymlinks: true,
            compressionLevel: 9,
        });
        const body = JSON.parse(
            (authFetchMock.mock.calls[0][1] as RequestInit).body as string,
        );
        expect(body.follow_symlinks).toBe(true);
        expect(body.compression_level).toBe(9);
    });

    it("reports progress as bytes flow through", async () => {
        const chunkA = new Uint8Array(1024);
        const chunkB = new Uint8Array(512);
        authFetchMock.mockResolvedValueOnce(
            streamFromChunks([chunkA, chunkB], {
                "X-Total-Bytes": "1536",
            }),
        );
        const progress: Array<{ received: number; total: number }> = [];
        await downloadArchive({
            projectId: "p1",
            agentId: "a1",
            paths: ["/x"],
            format: "tar",
            filename: "x.tar",
            onProgress: (received, total) => {
                progress.push({ received, total });
            },
        });
        expect(progress.length).toBeGreaterThanOrEqual(2);
        // Final progress reports the full total.
        const last = progress[progress.length - 1];
        expect(last.received).toBe(1536);
        expect(last.total).toBe(1536);
    });

    it("returns the transfer id from the response header", async () => {
        authFetchMock.mockResolvedValueOnce(
            streamFromChunks([new Uint8Array([1])], { "X-Transfer-Id": "ft-zoo" }),
        );
        const out = await downloadArchive({
            projectId: "p1",
            agentId: "a1",
            paths: ["/x"],
            format: "tar",
            filename: "x.tar",
        });
        expect(out.transferId).toBe("ft-zoo");
    });

    it("aborts the fetch when the AbortSignal fires", async () => {
        const abortSpy = vi.fn();
        authFetchMock.mockImplementationOnce(async (_path, init) => {
            const sig = (init as RequestInit).signal as AbortSignal | undefined;
            sig?.addEventListener("abort", abortSpy);
            // Return a response whose stream stalls forever so we
            // know the abort itself unwinds.
            const stream = new ReadableStream<Uint8Array>({
                async start() {
                    /* never closes */
                },
            });
            return new Response(stream, { status: 200 });
        });
        const ctrl = new AbortController();
        const promise = downloadArchive({
            projectId: "p1",
            agentId: "a1",
            paths: ["/x"],
            format: "tar",
            filename: "x.tar",
            signal: ctrl.signal,
        });
        ctrl.abort();
        await expect(promise).rejects.toBeDefined();
        expect(abortSpy).toHaveBeenCalled();
    });

    it("uses the File System Access API when available", async () => {
        const writes: Uint8Array[] = [];
        const writable = {
            write: vi.fn(async (chunk: Uint8Array) => {
                writes.push(chunk);
            }),
            close: vi.fn(async () => {}),
            abort: vi.fn(async () => {}),
        };
        const handle = {
            createWritable: vi.fn(async () => writable),
        };
        const showSaveFilePicker = vi.fn(async () => handle);
        // Inject the File System Access API for the duration of the test.
        const orig = (window as unknown as { showSaveFilePicker?: unknown })
            .showSaveFilePicker;
        (window as unknown as { showSaveFilePicker?: unknown }).showSaveFilePicker =
            showSaveFilePicker;
        try {
            authFetchMock.mockResolvedValueOnce(
                streamFromChunks([new Uint8Array([1, 2]), new Uint8Array([3, 4])]),
            );
            await downloadArchive({
                projectId: "p1",
                agentId: "a1",
                paths: ["/x"],
                format: "tar",
                filename: "x.tar",
            });
            expect(showSaveFilePicker).toHaveBeenCalled();
            expect(writes.length).toBe(2);
            expect(writable.close).toHaveBeenCalled();
        } finally {
            (window as unknown as { showSaveFilePicker?: unknown }).showSaveFilePicker =
                orig;
        }
    });

    it("falls back to a blob URL <a download> when FS Access API is missing", async () => {
        delete (window as unknown as { showSaveFilePicker?: unknown }).showSaveFilePicker;
        // Track URL.createObjectURL / revokeObjectURL.
        const created: Blob[] = [];
        const origCreate = URL.createObjectURL;
        const origRevoke = URL.revokeObjectURL;
        URL.createObjectURL = vi.fn((b: Blob) => {
            created.push(b);
            return "blob:fake";
        }) as typeof URL.createObjectURL;
        URL.revokeObjectURL = vi.fn();
        try {
            authFetchMock.mockResolvedValueOnce(
                streamFromChunks([new Uint8Array([1, 2, 3])]),
            );
            await downloadArchive({
                projectId: "p1",
                agentId: "a1",
                paths: ["/x"],
                format: "tar",
                filename: "x.tar",
            });
            expect(created.length).toBe(1);
        } finally {
            URL.createObjectURL = origCreate;
            URL.revokeObjectURL = origRevoke;
        }
    });

    it("rejects with the body text on a non-2xx response", async () => {
        const bad = new Response("permission denied", {
            status: 502,
            headers: { "Content-Type": "text/plain" },
        });
        authFetchMock.mockResolvedValueOnce(bad);
        await expect(
            downloadArchive({
                projectId: "p1",
                agentId: "a1",
                paths: ["/x"],
                format: "tar",
                filename: "x.tar",
            }),
        ).rejects.toThrow(/permission denied/);
    });
});
