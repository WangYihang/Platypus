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
            {/* offset=44 keeps toasts above the 28px StatusBar with
                a visible gap so the two regions read as separate
                elements instead of a stacked block. (The default
                offset of 32 plus a 56-ish-px toast still overlapped
                the bar in practice.) */}
            <Toaster position="bottom-right" offset={44} richColors closeButton />
        </TooltipProvider>
    </React.StrictMode>,
);
