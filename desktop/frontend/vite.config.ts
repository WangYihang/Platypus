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
    // wailsjs/go/models is committed (it's pure type definitions auto-
    // generated from the Go structs), so both build modes resolve to the
    // same file. Only App and runtime need a web-mode swap.
    const platformAliases = isWeb
        ? {
              "../wailsjs/go/app/App": path.resolve(__dirname, "src/platform/App.web.ts"),
              "../../wailsjs/go/app/App": path.resolve(__dirname, "src/platform/App.web.ts"),
              "../wailsjs/runtime/runtime": path.resolve(__dirname, "src/platform/runtime.web.ts"),
              "../../wailsjs/runtime/runtime": path.resolve(__dirname, "src/platform/runtime.web.ts"),
          }
        : {};

    return {
        plugins: [react()],
        resolve: { alias: platformAliases },
        build: {
            outDir: isWeb ? "dist-web" : "dist",
            // Manual vendor chunking. Splits the heavy third-party libs
            // (antd, recharts, xterm, react-router) into their own
            // cacheable chunks so the pages that don't need them don't
            // pay the bytes, and so a page update that leaves the
            // vendor graph unchanged only invalidates a small app
            // chunk. Route-level `React.lazy()` in routes.tsx handles
            // the per-page split; this handles the shared graph.
            //
            // The antd icon set and the antd core are split apart even
            // though both are technically shared deps: icons are huge
            // (each `@ant-design/icons` entry is its own module) and
            // don't change often, while antd core ships breaking
            // changes at a normal cadence. Keeping them as separate
            // chunks lets a bump to one invalidate only that cache.
            rolldownOptions: {
                output: {
                    manualChunks(id: string) {
                        if (!id.includes("node_modules")) return undefined;
                        if (id.includes("/@ant-design/icons")) return "vendor-antd-icons";
                        if (
                            id.includes("/recharts/") ||
                            id.includes("/victory-vendor/") ||
                            id.includes("/d3-")
                        ) {
                            return "vendor-charts";
                        }
                        if (id.includes("/@xterm/")) return "vendor-xterm";
                        if (id.includes("/antd/") || id.includes("/rc-")) return "vendor-antd";
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
            // antd is the whole component library; its chunk is ~1 MB
            // (~300 KB gzipped) by design and is shared across every
            // route, so the default 500 KB warning threshold is noise
            // here. Lift it just high enough that a real regression
            // (e.g. a page accidentally bundling a huge dep) still
            // trips the warning.
            chunkSizeWarningLimit: 1200,
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
