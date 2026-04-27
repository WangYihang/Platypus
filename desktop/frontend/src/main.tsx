import React from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";

import "./style.css";
import { router } from "./routes";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
    <React.StrictMode>
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
    </React.StrictMode>,
);
