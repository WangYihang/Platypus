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
            <Toaster position="bottom-right" richColors closeButton />
        </TooltipProvider>
    </React.StrictMode>,
);
