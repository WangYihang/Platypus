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
            {/* Sonner v2 measures `offset` from the viewport edge to the
                toast container, but the rendered <li> picks up an
                additional ~13px of internal viewport padding/transforms
                that the spec accounts for via boundingBox. To leave a
                visible >=8px gap above the 28px StatusBar we need the
                effective offset (offset - StatusBar height) to clear
                that ~13px overhead too: 60 - 28 = 32, minus ~13 internal
                = ~19px gap, comfortably above the 8px floor without
                pushing the toast away from the corner. */}
            <Toaster position="bottom-right" offset={60} richColors closeButton />
        </TooltipProvider>
    </React.StrictMode>,
);
