import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
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
    // wailsjs/go/models is committed (it's pure type definitions auto-
    // generated from the Go structs), so both build modes resolve to the
    // same file. Only App and runtime need a web-mode swap.
    //
    // Use regex aliases to match the Wails paths at any import depth.
    const platformAliases = isWeb
        ? [
              {
                  find: /.*\/wailsjs\/go\/app\/App$/,
                  replacement: path.resolve(__dirname, "src/platform/App.web.ts"),
              },
              {
                  find: /.*\/wailsjs\/runtime\/runtime$/,
                  replacement: path.resolve(__dirname, "src/platform/runtime.web.ts"),
              },
          ]
        : [];

    return {
        plugins: [react(), tailwindcss()],
        resolve: {
            alias: [
                ...(Array.isArray(platformAliases) ? platformAliases : []),
                { find: "@", replacement: path.resolve(__dirname, "src") },
            ],
        },
        build: {
            outDir: isWeb ? "dist-web" : "dist",
            // Manual vendor chunking. Splits the heavy third-party libs
            // into their own cacheable chunks so the pages that don't
            // need them don't pay the bytes, and so a page update that
            // leaves the vendor graph unchanged only invalidates a
            // small app chunk. Route-level `React.lazy()` in routes.tsx
            // handles the per-page split; this handles the shared
            // graph.
            //
            // vendor-react / vendor-router are always on the hot path
            // (every route mounts them). vendor-xterm is only fetched
            // when the Host terminal tab opens; vendor-charts only when
            // ProjectOverview does. Everything else (Radix primitives,
            // react-hook-form, zod, lucide-react, date-fns, …) lands
            // in vendor-misc.
            rolldownOptions: {
                output: {
                    manualChunks(id: string) {
                        if (!id.includes("node_modules")) return undefined;
                        if (
                            id.includes("/recharts/") ||
                            id.includes("/victory-vendor/") ||
                            id.includes("/d3-")
                        ) {
                            return "vendor-charts";
                        }
                        if (id.includes("/@xterm/")) return "vendor-xterm";
                        if (id.includes("/react-router")) return "vendor-router";
                        if (
                            id.includes("/react/") ||
                            id.includes("/react-dom/") ||
                            id.includes("/scheduler/")
                        ) {
                            return "vendor-react";
                        }
                        return "vendor-misc";
                    },
                },
            },
            // Default 500 KB chunk-size warning. With antd gone the
            // largest shared chunk is vendor-charts at ~370 KB raw; any
            // new regression pushing a chunk past that probably deserves
            // a second look.
        },
        // Build-time globals consumed by the status bar. Set GIT_COMMIT in
        // CI (or via the Makefile) to get a real short hash; falls back to
        // "dev" for local unreleased builds.
        define: {
            __APP_VERSION__: JSON.stringify(process.env.npm_package_version ?? "dev"),
            __APP_COMMIT__: JSON.stringify(process.env.GIT_COMMIT ?? "dev"),
        },
    };
});
