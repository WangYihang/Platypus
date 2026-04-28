import { describe, expect, it, vi, beforeEach } from "vitest";

// file-upload.ts is the streaming-upload primitive used by the
// drag-drop file browser. Wraps a single PUT to /fs/upload with
// progress callbacks (server pushes WS events with bytes_transferred;
// the lib also reports local bytes-sent so the file browser can show
// progress before the first WS tick lands), AbortSignal for cancel,
// and a returned X-Transfer-Id so the UI can correlate later WS
// events with this upload.

vi.mock("./auth", () => ({
    authFetch: vi.fn(),
    getSession: () => ({ serverURL: "https://example.test", sessionToken: "tok" }),
}));

import { authFetch } from "./auth";
import { uploadFile } from "./file-upload";

const authFetchMock = vi.mocked(authFetch);

beforeEach(() => {
    authFetchMock.mockReset();
});

function fakeFile(name: string, bytes: number): File {
    return new File([new Uint8Array(bytes)], name, { type: "application/octet-stream" });
}

describe("uploadFile", () => {
    it("PUTs the file to the project-scoped /fs/upload endpoint with size in query", async () => {
        authFetchMock.mockResolvedValueOnce(
            new Response(
                JSON.stringify({ bytes_written: 1024, transfer_id: "ft-up-1" }),
                {
                    status: 200,
                    headers: {
                        "Content-Type": "application/json",
                        "X-Transfer-Id": "ft-up-1",
                    },
                },
            ),
        );
        const f = fakeFile("data.bin", 1024);
        const out = await uploadFile({
            projectId: "p1",
            agentId: "a1",
            remoteDir: "/var/log",
            file: f,
        });
        expect(out.bytesWritten).toBe(1024);
        expect(out.transferId).toBe("ft-up-1");

        const [path, init] = authFetchMock.mock.calls[0];
        expect(path).toContain("/api/v1/projects/p1/agents/a1/fs/upload");
        expect(path).toContain("path=");
        expect(path).toContain("total_bytes=1024");
        expect(path).toContain("mkdirs=true");
        expect(init?.method).toBe("PUT");
        expect((init?.headers as Record<string, string>)["Content-Type"]).toBe(
            "application/octet-stream",
        );
    });

    it("URL-encodes the destination path", async () => {
        authFetchMock.mockResolvedValueOnce(
            new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
        );
        const f = fakeFile("config file.txt", 4);
        await uploadFile({
            projectId: "p1",
            agentId: "a1",
            remoteDir: "/etc/foo bar",
            file: f,
        });
        const path = authFetchMock.mock.calls[0][0] as string;
        // %2F for "/", %20 for spaces.
        expect(path).toContain("path=%2Fetc%2Ffoo%20bar%2Fconfig%20file.txt");
    });

    it("sends the file body as the request body", async () => {
        let receivedBody: BodyInit | null = null;
        authFetchMock.mockImplementationOnce(async (_p, init) => {
            receivedBody = (init as RequestInit).body ?? null;
            return new Response("{}", {
                status: 200,
                headers: { "Content-Type": "application/json" },
            });
        });
        const f = fakeFile("x.bin", 7);
        await uploadFile({ projectId: "p1", agentId: "a1", remoteDir: "/d", file: f });
        // The body is the File itself (or a Blob built from it) so the
        // browser can stream it without us reading bytes into memory.
        expect(receivedBody).not.toBeNull();
    });

    it("reports a final 100% progress via onProgress when no WS event arrives", async () => {
        authFetchMock.mockResolvedValueOnce(
            new Response(
                JSON.stringify({ bytes_written: 16, transfer_id: "ft-1" }),
                { status: 200, headers: { "Content-Type": "application/json" } },
            ),
        );
        const progress: Array<{ received: number; total: number }> = [];
        const f = fakeFile("y.bin", 16);
        await uploadFile({
            projectId: "p1",
            agentId: "a1",
            remoteDir: "/d",
            file: f,
            onProgress: (received, total) => progress.push({ received, total }),
        });
        // At minimum the final progress tick fires with full bytes.
        expect(progress.length).toBeGreaterThanOrEqual(1);
        const last = progress[progress.length - 1];
        expect(last.received).toBe(16);
        expect(last.total).toBe(16);
    });

    it("rejects with the body text on a non-2xx response", async () => {
        authFetchMock.mockResolvedValueOnce(
            new Response("permission denied", {
                status: 502,
                headers: { "Content-Type": "text/plain" },
            }),
        );
        const f = fakeFile("z.bin", 4);
        await expect(
            uploadFile({ projectId: "p1", agentId: "a1", remoteDir: "/d", file: f }),
        ).rejects.toThrow(/permission denied/);
    });

    it("threads AbortSignal through to the underlying fetch", async () => {
        authFetchMock.mockImplementationOnce(async (_p, init) => {
            const sig = (init as RequestInit).signal as AbortSignal | undefined;
            // If aborted by the time fetch is called, simulate the
            // browser's behaviour: throw immediately.
            if (sig?.aborted) {
                throw new DOMException("aborted", "AbortError");
            }
            return new Response("{}", {
                status: 200,
                headers: { "Content-Type": "application/json" },
            });
        });
        const ctrl = new AbortController();
        ctrl.abort();
        const f = fakeFile("a.bin", 2);
        await expect(
            uploadFile({
                projectId: "p1",
                agentId: "a1",
                remoteDir: "/d",
                file: f,
                signal: ctrl.signal,
            }),
        ).rejects.toThrow();
    });
});
