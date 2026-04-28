import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

// Smoke test for the QueryClient setup that R3 builds on. We don't
// re-test what tanstack themselves test (caching, dedup, retry) —
// just pin the contract that `useQuery` resolves a mocked queryFn,
// surfaces loading → success transitions, and that `refetch` re-runs
// the fn.
//
// If this spec breaks, the full react-query migration is
// fundamentally broken — every page-level useQuery shares the same
// machinery exercised here.

function makeWrapper(client: QueryClient) {
    return function Wrapper({ children }: { children: ReactNode }) {
        return (
            <QueryClientProvider client={client}>
                {children}
            </QueryClientProvider>
        );
    };
}

function freshClient() {
    return new QueryClient({
        defaultOptions: { queries: { retry: false } },
    });
}

describe("react-query smoke", () => {
    it("resolves a mocked queryFn and exposes data", async () => {
        const fn = vi.fn().mockResolvedValue({ hello: "world" });
        const wrapper = makeWrapper(freshClient());

        const { result } = renderHook(
            () =>
                useQuery({
                    queryKey: ["smoke", "happy"],
                    queryFn: fn,
                }),
            { wrapper },
        );

        // Initial render is loading.
        expect(result.current.isLoading).toBe(true);
        expect(result.current.data).toBeUndefined();

        await waitFor(() => expect(result.current.isLoading).toBe(false));
        expect(result.current.data).toEqual({ hello: "world" });
        expect(fn).toHaveBeenCalledTimes(1);
    });

    it("refetch re-invokes the queryFn", async () => {
        const fn = vi.fn().mockResolvedValue("ok");
        const wrapper = makeWrapper(freshClient());

        const { result } = renderHook(
            () =>
                useQuery({
                    queryKey: ["smoke", "refetch"],
                    queryFn: fn,
                }),
            { wrapper },
        );

        await waitFor(() => expect(result.current.isSuccess).toBe(true));
        expect(fn).toHaveBeenCalledTimes(1);

        await result.current.refetch();
        expect(fn).toHaveBeenCalledTimes(2);
    });

    it("surfaces errors via the error slot", async () => {
        const fn = vi.fn().mockRejectedValue(new Error("nope"));
        const wrapper = makeWrapper(freshClient());

        const { result } = renderHook(
            () =>
                useQuery({
                    queryKey: ["smoke", "err"],
                    queryFn: fn,
                }),
            { wrapper },
        );

        await waitFor(() => expect(result.current.isError).toBe(true));
        expect(String(result.current.error)).toContain("nope");
    });
});
