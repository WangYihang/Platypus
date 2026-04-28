import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, RenderOptions, RenderResult } from "@testing-library/react";
import { ReactElement, ReactNode } from "react";

// renderWithQueryClient wraps a component under a fresh QueryClient
// so any `useQuery` / `useQueryClient` consumer doesn't blow up with
// "No QueryClient set" in vitest. Each call creates its own client
// so tests don't bleed cache across each other.
//
// Usage:
//     import { renderWithQueryClient } from "../testing/renderWithQueryClient";
//     renderWithQueryClient(<ProjectMembers project={project} />);

export function renderWithQueryClient(
    ui: ReactElement,
    options?: RenderOptions,
): RenderResult {
    const client = new QueryClient({
        defaultOptions: {
            queries: {
                retry: false,
                // Disable refetch-on-mount churn in tests so a single
                // assertion sequence doesn't have to race a refetch.
                refetchOnMount: false,
                refetchOnWindowFocus: false,
            },
        },
    });
    function Wrapper({ children }: { children: ReactNode }) {
        return (
            <QueryClientProvider client={client}>{children}</QueryClientProvider>
        );
    }
    return render(ui, { wrapper: Wrapper, ...options });
}
