import { ConsoleMessage, test as base, expect } from "@playwright/test";

// Shared test base that attaches a console watcher to every page. Any
// browser console.error / console.warn or uncaught page error during
// the test fails it at teardown — so silent UI regressions (e.g. the
// "useGlobalTerminal outside provider" throw that Error Boundary
// caught but still logged) can't slip past a green assertion pass.
//
// Swap the `@playwright/test` import for this file in every spec.

interface Capture {
    type: string;
    text: string;
    location?: string;
}

// Known-benign messages we deliberately let through. Keep tiny; prefer
// fixing the root cause over silencing. Each entry needs a comment
// explaining WHY it's acceptable.
const IGNORE_PATTERNS: RegExp[] = [
    // React DevTools install hint — always logged in dev mode, never
    // actionable. https://reactjs.org/link/react-devtools
    /Download the React DevTools/i,
    // Vite HMR handshake & plugin chatter — only surfaces in dev mode
    // and is not actionable in tests.
    /\[vite\]/i,
];

function formatMsg(msg: ConsoleMessage): Capture {
    return {
        type: msg.type(),
        text: msg.text(),
        location: msg.location()?.url,
    };
}

function isIgnored(m: Capture): boolean {
    return IGNORE_PATTERNS.some((re) => re.test(m.text));
}

export const test = base.extend<{ quietConsole: void }>({
    quietConsole: [
        async ({ page }, use, testInfo) => {
            const captured: Capture[] = [];

            page.on("console", (msg) => {
                const level = msg.type();
                if (level !== "error" && level !== "warning") return;
                captured.push(formatMsg(msg));
            });
            page.on("pageerror", (err) => {
                captured.push({
                    type: "pageerror",
                    text: `${err.name}: ${err.message}`,
                });
            });

            await use();

            const noise = captured.filter((m) => !isIgnored(m));
            if (noise.length > 0) {
                const lines = noise.map(
                    (m) => `  [${m.type}] ${m.text}${m.location ? ` (${m.location})` : ""}`,
                );
                testInfo.annotations.push({
                    type: "console-noise",
                    description: lines.join("\n"),
                });
                throw new Error(
                    `Browser emitted ${noise.length} console error/warning:\n${lines.join("\n")}`,
                );
            }
        },
        { auto: true },
    ],
});

export { expect };
