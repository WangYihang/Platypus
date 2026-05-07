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
                        // raw; only FileEditor uses it. `/@uiw/` (not just
                        // /@uiw/react-codemirror/) catches the full kit
                        // including codemirror-extensions-basic-setup
                        // which ships ~390 KB on its own.
                        if (
                            id.includes("/@codemirror/") ||
                            id.includes("/@uiw/") ||
                            id.includes("/@lezer/") ||
                            id.includes("/style-mod/") ||
                            id.includes("/w3c-keyname/") ||
                            id.includes("/crelt/")
                        ) {
                            return "vendor-editor";
                        }
                        // pdfjs-dist's main lib + the react-pdf wrapper
                        // (~420 KB combined) are route-lazy via PdfViewer.
                        // Pin both to vendor-pdf so they don't bleed into
                        // the eager catch-all when the manualChunks
                        // function force-attributes them.
                        if (
                            id.includes("/pdfjs-dist/") ||
                            id.includes("/react-pdf/")
                        ) {
                            return "vendor-pdf";
                        }
                        // Radix primitives are spread across ~30 packages
                        // that aggregate to ~250 KB. They're imported
                        // app-wide; pinning them to one chunk gives a
                        // stable filename + shrinks vendor-misc.
                        if (id.includes("/@radix-ui/")) return "vendor-radix";
                        // Icons — per-icon imports tree-shake fine but
                        // the survivors accumulate to ~150-300 KB across
                        // 225+ import sites. One chunk = stable filename
                        // (browser-cache-friendly across deploys that
                        // don't touch icons) + identifiable on the
                        // network panel.
                        if (id.includes("/lucide-react/")) return "vendor-icons";
                        // TanStack Query: singleton in main.tsx, used by
                        // ~20 pages. Always-on but ~50-80 KB.
                        if (
                            id.includes("/@tanstack/react-query/") ||
                            id.includes("/@tanstack/query-core/") ||
                            id.includes("/@tanstack/react-query-devtools/")
                        ) {
                            return "vendor-query";
                        }
                        // i18next + locale loaders, bootstrapped before
                        // render (must stay eager).
                        if (
                            id.includes("/i18next/") ||
                            id.includes("/react-i18next/") ||
                            id.includes("/i18next-browser-languagedetector/")
                        ) {
                            return "vendor-i18n";
                        }
                        // Form validation stack — Login (route-lazy) +
                        // Add* dialogs (lazy via Suspense in ProjectShell).
                        if (
                            id.includes("/react-hook-form/") ||
                            id.includes("/@hookform/resolvers/") ||
                            id.includes("/zod/")
                        ) {
                            return "vendor-forms";
                        }
                        if (
                            id.includes("/react/") ||
                            id.includes("/react-dom/") ||
                            id.includes("/scheduler/")
                        ) {
                            return "vendor-react";
                        }
                        // Default: let Vite decide. Returning undefined
                        // means modules reachable only through lazy
                        // routes ride along with those lazy chunks
                        // instead of getting force-pulled into a
                        // catch-all "vendor-misc" that ships eagerly.
                        // The named splits above are libs we KNOW must
                        // be eager (or want stable filenames for).
                        return undefined;
                    },
                },
            },
            // vendor-misc + vendor-editor are the natural ceilings here
            // after splitting out cytoscape (vendor-graph), pdfjs
            // (vendor-pdf), radix (vendor-radix), icons (vendor-icons),
            // query (vendor-query), i18n (vendor-i18n), and forms
            // (vendor-forms). Anything above 600 KB raw is a regression
            // worth investigating. The pdf.worker.mjs chunk is a Web
            // Worker (separate execution context), so its size doesn't
            // count against the main-bundle perf budget.
            chunkSizeWarningLimit: 600,
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
