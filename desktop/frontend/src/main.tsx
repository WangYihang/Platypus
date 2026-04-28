import React from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";

import "./style.css";
import { router } from "./routes";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

// Single QueryClient for the whole app — every page uses it via
// `useQuery` / `useInfiniteQuery` so cross-page reads share the
// cache (e.g. opening a host then bouncing back to Fleet doesn't
// re-fetch the host row). Defaults are tuned for an admin-console
// workload: 30 s stale time keeps fresh data fresh without
// hammering the API; refetch-on-focus picks up server-side changes
// after the operator alt-tabs back in; one retry is enough for a
// transient network blip but doesn't mask real outages.
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30_000,
            refetchOnWindowFocus: true,
            retry: 1,
        },
    },
});

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
    <React.StrictMode>
        <QueryClientProvider client={queryClient}>
            {/* TooltipProvider is a no-cost wrapper that lets any shadcn
                Tooltip below render without needing a local provider.
                Toaster renders the sonner portal for toast.* calls. */}
            <TooltipProvider delayDuration={200}>
                <RouterProvider router={router} />
                {/* Sonner v2 measures `offset` from the viewport edge to
                    the toast container, but the rendered <li> picks up
                    additional internal padding/transforms that the
                    boundingBox-based 34-toast-statusbar-overlap spec
                    measures. The exact loss has drifted with Chromium
                    point-versions: started at ~13px (offset 44 → gap
                    ~3px, F4 audit), we then went to offset 60 → gap ~19px
                    stable. After a transitive lockfile refresh in 3ab377c
                    the loss grew to ~32px, which collapsed the previous
                    comfortable 19px gap into 0.37px and re-tripped the
                    spec.

                    Lift the offset to 88 so even with the new internal
                    overhead the effective gap stays comfortably above
                    the 8px AA floor:
                        88 (offset) − 28 (StatusBar) − ~32 (internal)
                      = ~28px gap. */}
                <Toaster position="bottom-right" offset={88} richColors closeButton />
            </TooltipProvider>
            {import.meta.env.DEV && <ReactQueryDevtools initialIsOpen={false} />}
        </QueryClientProvider>
    </React.StrictMode>,
);
