import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

// vitest.config.ts is intentionally separate from vite.config.ts so the
// build pipeline (which knows about Wails alias resolution and chunking)
// stays orthogonal to the test runner. Tests always resolve "@wails/*"
// to the platform shims in src/platform — same as `tsc --noEmit` does
// via tsconfig.json paths — so no Wails-generated code is ever loaded
// from a test, regardless of the vite mode.
export default defineConfig({
    plugins: [react()],
    resolve: {
        alias: [
            {
                find: "@wails/go/app/App",
                replacement: path.resolve(__dirname, "src/platform/App.web.ts"),
            },
            {
                find: "@wails/runtime/runtime",
                replacement: path.resolve(__dirname, "src/platform/runtime.web.ts"),
            },
            { find: "@", replacement: path.resolve(__dirname, "src") },
        ],
    },
    test: {
        environment: "jsdom",
        globals: false,
        setupFiles: ["./src/test/setup.ts"],
        include: ["src/**/*.{test,spec}.{ts,tsx}"],
    },
    // Build-time globals that production code reads via vite's
    // `define`. Without these tests crash with
    // "ReferenceError: __APP_VERSION__ is not defined" the moment
    // they touch StatusBar (or any component that reads them).
    // Lives at the top level — `test.define` is silently ignored.
    define: {
        __APP_VERSION__: JSON.stringify("test"),
        __APP_COMMIT__: JSON.stringify("test"),
    },
});
