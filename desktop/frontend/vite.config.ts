import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

// Two build modes share one source tree:
//
//  - default (Wails)          → @wails/* resolves to the bindings
//                                `wails generate module` writes under
//                                desktop/frontend/wailsjs/. Output:
//                                dist/ (consumed by `wails build`).
//  - --mode web (standalone)  → @wails/* resolves to hand-written
//                                REST/WebSocket shims under src/platform/.
//                                Output: dist-web/ (static bundle the user
//                                serves wherever they want).
//
// No page-level code branches on the mode — every page imports from
// "@wails/go/app/App" / "@wails/runtime/runtime" and the alias below
// picks the right backend. tsconfig.json paths point @wails/* at the
// platform shims unconditionally so `tsc` type-checks both modes
// without the wailsjs/ tree having to exist on disk.
export default defineConfig(({ mode }) => {
    const isWeb = mode === "web";
    const platformAliases = isWeb
        ? [
              {
                  find: "@wails/go/app/App",
                  replacement: path.resolve(__dirname, "src/platform/App.web.ts"),
              },
              {
                  find: "@wails/runtime/runtime",
                  replacement: path.resolve(__dirname, "src/platform/runtime.web.ts"),
              },
          ]
        : [
              {
                  find: "@wails/go/app/App",
                  replacement: path.resolve(__dirname, "wailsjs/go/app/App"),
              },
              {
                  find: "@wails/runtime/runtime",
                  replacement: path.resolve(__dirname, "wailsjs/runtime/runtime"),
              },
          ];

    return {
        plugins: [react(), tailwindcss()],
        resolve: {
            alias: [
                ...platformAliases,
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
                        // asciinema-player (~65 KB gz incl. inlined
                        // WASM) is only loaded when the Recordings
                        // preview opens — pin it to its own named
                        // chunk so it ships separately from the
                        // always-on vendor-misc bundle and gets a
                        // recognisable filename.
                        if (id.includes("/asciinema-player/")) return "vendor-asciinema";
                        // Cytoscape ships ~900 KB raw; split into its own
                        // chunk so only the Topology/Graph view on Fleet
                        // pays for it.
                        if (
                            id.includes("/cytoscape/") ||
                            id.includes("/cytoscape-fcose/") ||
                            id.includes("/layout-base/") ||
                            id.includes("/cose-base/")
                        ) {
                            return "vendor-graph";
                        }
                        // CodeMirror (editor + language modes) is ~300 KB
                        // raw; only FileEditor uses it.
                        if (
                            id.includes("/@codemirror/") ||
                            id.includes("/@uiw/react-codemirror/") ||
                            id.includes("/@lezer/") ||
                            id.includes("/style-mod/") ||
                            id.includes("/w3c-keyname/") ||
                            id.includes("/crelt/")
                        ) {
                            return "vendor-editor";
                        }
                        // pdfjs-dist's main lib (~600 KB) is route-lazy
                        // (PdfViewer only). Splitting it out keeps it
                        // off the always-on vendor-misc bundle.
                        if (id.includes("/pdfjs-dist/")) return "vendor-pdf";
                        // Radix primitives are spread across ~30 packages
                        // that aggregate to ~250 KB. They're imported
                        // app-wide; pinning them to one chunk gives a
                        // stable filename + shrinks vendor-misc.
                        if (id.includes("/@radix-ui/")) return "vendor-radix";
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
            // vendor-misc + vendor-editor are the natural ceilings here
            // after splitting out cytoscape (vendor-graph), pdfjs
            // (vendor-pdf), and radix (vendor-radix). Both top out
            // around ~700 KB raw / ~200 KB gzipped — anything bigger
            // is a regression worth investigating. The pdf.worker.mjs
            // chunk is a Web Worker (separate execution context), so
            // its size doesn't count against the main bundle.
            chunkSizeWarningLimit: 800,
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
