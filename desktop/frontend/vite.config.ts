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
            // No manualChunks — Vite/rolldown already produces
            // sensible chunks once routes use React.lazy (routes.tsx).
            // Lazy-only deps ride along with their lazy chunks; eager
            // deps consolidate into the entry. Adding manualChunks
            // rules previously force-pulled lazy libs into the eager
            // bundle via a catch-all "vendor-misc" — net negative.
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
