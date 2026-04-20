import React from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
import App from "./App";
import WebShell from "./WebShell";

// vite builds with --mode web set import.meta.env.MODE = "web". In that
// mode the wailsjs-path aliases point at the fetch-based platform shim
// (see vite.config.ts) and the root component is WebShell, which gates
// on localStorage before handing off to the tabbed App.
const Root = import.meta.env.MODE === "web" ? WebShell : App;

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
    <React.StrictMode>
        <Root />
    </React.StrictMode>
);
