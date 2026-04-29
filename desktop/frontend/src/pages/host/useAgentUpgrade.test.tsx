import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactNode } from "react";

// sonner's toast is fired from inside the mutation callbacks. Mock
// it so we can assert which variant rendered without touching the
// DOM. vi.hoisted ensures the spy registry is created BEFORE
// vi.mock runs (which itself is hoisted to the top of the file).
const toastMocks = vi.hoisted(() => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
}));
vi.mock("sonner", () => ({
    toast: toastMocks,
}));

// Mock the API surface — the hook is the unit under test, the
// network layer isn't.
vi.mock("../../lib/api", () => ({
    triggerAgentUpgrade: vi.fn(),
}));

import { triggerAgentUpgrade } from "../../lib/api";
import { useAgentUpgradeMutation } from "./useAgentUpgrade";
import { qk } from "../../lib/queryKeys";

const triggerMock = vi.mocked(triggerAgentUpgrade);

function wrapWithClient(client: QueryClient) {
    return function Wrapper({ children }: { children: ReactNode }) {
        return (
            <QueryClientProvider client={client}>{children}</QueryClientProvider>
        );
    };
}

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
}

beforeEach(() => {
    vi.clearAllMocks();
});

describe("useAgentUpgradeMutation", () => {
    it("toasts success and invalidates host queries on `exited`", async () => {
        triggerMock.mockResolvedValueOnce({
            status: "exited",
            phase: "PHASE_EXITING",
            resolved_version: "1.6.0",
        });
        const client = makeClient();
        const invalidateSpy = vi.spyOn(client, "invalidateQueries");

        const { result } = renderHook(() => useAgentUpgradeMutation("p1", "h1"), {
            wrapper: wrapWithClient(client),
        });

        act(() => {
            result.current.mutate({ target_version: "1.6.0", channel: "stable" });
        });
        await waitFor(() => expect(result.current.isSuccess).toBe(true));

        expect(triggerMock).toHaveBeenCalledWith("p1", "h1", {
            target_version: "1.6.0",
            channel: "stable",
        });
        expect(toastMocks.success).toHaveBeenCalledTimes(1);
        // resolved_version surfaces in the toast so operators see the
        // version they installed without checking the activity log.
        expect(toastMocks.success.mock.calls[0][0]).toMatch(/1\.6\.0/);

        // Hosts list + host detail both refresh so the build_version
        // row repaints once the agent reconnects under the new
        // binary. Drift here is exactly the kind of bug the hook was
        // extracted to prevent — assert the keys explicitly.
        expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: qk.hosts("p1") });
        expect(invalidateSpy).toHaveBeenCalledWith({
            queryKey: qk.host("p1", "h1"),
        });
    });

    it("toasts error with code+message when the agent reports `failed`", async () => {
        triggerMock.mockResolvedValueOnce({
            status: "failed",
            phase: "PHASE_FAILED",
            error_code: "signature_mismatch",
            error_message: "manifest sig did not verify",
        });
        const client = makeClient();
        const { result } = renderHook(() => useAgentUpgradeMutation("p1", "h1"), {
            wrapper: wrapWithClient(client),
        });

        act(() => {
            result.current.mutate({});
        });
        await waitFor(() => expect(result.current.isSuccess).toBe(true));

        // The mutation itself succeeded — the agent just reported a
        // problem. Toast routes through .error() so the operator
        // notices, but no Error gets thrown into onError.
        expect(toastMocks.error).toHaveBeenCalledTimes(1);
        const msg = toastMocks.error.mock.calls[0][0] as string;
        expect(msg).toContain("signature_mismatch");
        expect(msg).toContain("manifest sig did not verify");
    });

    it("toasts info on `in_progress` (server timed out waiting for terminal frame)", async () => {
        triggerMock.mockResolvedValueOnce({
            status: "in_progress",
            phase: "PHASE_DOWNLOAD",
            bytes_done: 1024,
            bytes_total: 50_000_000,
        });
        const client = makeClient();
        const { result } = renderHook(() => useAgentUpgradeMutation("p1", "h1"), {
            wrapper: wrapWithClient(client),
        });
        act(() => {
            result.current.mutate({});
        });
        await waitFor(() => expect(result.current.isSuccess).toBe(true));

        expect(toastMocks.info).toHaveBeenCalledTimes(1);
    });

    it("routes a thrown error through onError (e.g. agent-not-connected 404)", async () => {
        triggerMock.mockRejectedValueOnce(new Error("404: agent not connected"));
        const client = makeClient();
        const { result } = renderHook(() => useAgentUpgradeMutation("p1", "h1"), {
            wrapper: wrapWithClient(client),
        });

        act(() => {
            result.current.mutate({});
        });
        await waitFor(() => expect(result.current.isError).toBe(true));

        expect(toastMocks.error).toHaveBeenCalledTimes(1);
    });
});
