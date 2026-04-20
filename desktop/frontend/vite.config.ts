import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// Two build modes share one source tree:
//
//  - default (Wails)          → imports resolve to wailsjs/go/app/App,
//                                wailsjs/runtime/runtime, wailsjs/go/models.
//                                Output: dist/ (consumed by `wails build`).
//  - --mode web (standalone)  → same imports get aliased to hand-written
//                                REST/WebSocket shims under src/platform/.
//                                Output: dist-web/ (static bundle the user
//                                serves wherever they want).
//
// No page-level code branches on the mode — every page keeps importing
// from "../../wailsjs/go/app/App" etc. The alias below is the whole trick.
export default defineConfig(({ mode }) => {
    const isWeb = mode === "web";
    const platformAliases = isWeb
        ? {
              "../wailsjs/go/app/App": path.resolve(__dirname, "src/platform/App.web.ts"),
              "../../wailsjs/go/app/App": path.resolve(__dirname, "src/platform/App.web.ts"),
              "../wailsjs/runtime/runtime": path.resolve(__dirname, "src/platform/runtime.web.ts"),
              "../../wailsjs/runtime/runtime": path.resolve(__dirname, "src/platform/runtime.web.ts"),
              "../wailsjs/go/models": path.resolve(__dirname, "src/platform/models.web.ts"),
              "../../wailsjs/go/models": path.resolve(__dirname, "src/platform/models.web.ts"),
          }
        : {};

    return {
        plugins: [react()],
        resolve: { alias: platformAliases },
        build: { outDir: isWeb ? "dist-web" : "dist" },
    };
});
