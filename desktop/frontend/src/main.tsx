import React from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
import WebShell from "./WebShell";

// Both desktop (Wails) and web builds render WebShell. WebShell handles
// JWT auth (Login → Workspace) over HTTP; the Wails build is just a
// packaging shell around the same UI. The legacy Wails-binding tabs
// (Sessions/Listeners/Files/Tunnels/Connect) were removed; if profile
// management is reintroduced for desktop it should live inside
// ProfileRail, not as a separate pre-shell.
const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
    <React.StrictMode>
        <WebShell />
    </React.StrictMode>
);
