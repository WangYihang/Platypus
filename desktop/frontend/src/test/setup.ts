// Vitest global test setup. Loaded once per test worker via the
// `setupFiles` entry in vitest.config.ts.
//
// What lives here:
//   · @testing-library/jest-dom — registers the toBeInTheDocument /
//     toHaveTextContent matchers on vitest's expect, so component tests
//     can assert against the rendered DOM in the same vocabulary the
//     rest of the React ecosystem uses.
//   · matchMedia stub — Radix primitives (Dialog, Popover, Tooltip)
//     read window.matchMedia synchronously in jsdom, which doesn't
//     ship with one. Returning an "always false" media query is fine
//     for tests; specs that need a specific breakpoint can override it.
//   · ResizeObserver stub — recharts and Radix size their containers
//     via ResizeObserver. jsdom doesn't implement it, and a missing
//     constructor throws at module-load time.

import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// i18n: tests render real components that call useTranslation. The
// production app initialises the i18next instance in main.tsx via a
// side-effect import; tests never load main.tsx. Pin every spec to
// en-US so a developer with `lang=zh-CN` already in their
// localStorage doesn't see English-string assertions break — the
// LanguageDetector chain reads localStorage and we want a
// deterministic value here regardless of the host environment.
import i18n, { i18nReady } from "../i18n";

await i18nReady;
await i18n.changeLanguage("en-US");

// RTL's auto-cleanup hook only registers if a global `afterEach` is in
// scope. Vitest only injects globals when `globals: true` is set in
// the config — we run with `globals: false` so the imports stay
// explicit, which means the auto-cleanup never fires and successive
// renders inside the same file accumulate in the DOM. Run cleanup
// manually so each test starts from an empty document.
afterEach(() => {
    cleanup();
});

if (typeof window !== "undefined") {
    if (!window.matchMedia) {
        window.matchMedia = (query: string) =>
            ({
                matches: false,
                media: query,
                onchange: null,
                addEventListener: () => {},
                removeEventListener: () => {},
                addListener: () => {},
                removeListener: () => {},
                dispatchEvent: () => false,
            } as unknown as MediaQueryList);
    }
    if (!("ResizeObserver" in window)) {
        class ResizeObserverStub {
            observe() {}
            unobserve() {}
            disconnect() {}
        }
        (window as unknown as { ResizeObserver: typeof ResizeObserverStub }).ResizeObserver =
            ResizeObserverStub;
    }
    if (!("IntersectionObserver" in window)) {
        // Fires the callback eagerly with `isIntersecting: true` so any
        // component gated on visibility (e.g. Thumbnail) loads in tests
        // without the test having to scroll a fake viewport.
        class IntersectionObserverStub {
            constructor(private cb: IntersectionObserverCallback) {}
            observe(target: Element) {
                queueMicrotask(() => {
                    this.cb(
                        [
                            {
                                isIntersecting: true,
                                target,
                                intersectionRatio: 1,
                                time: 0,
                                boundingClientRect: {} as DOMRectReadOnly,
                                intersectionRect: {} as DOMRectReadOnly,
                                rootBounds: null,
                            },
                        ],
                        this as unknown as IntersectionObserver,
                    );
                });
            }
            unobserve() {}
            disconnect() {}
            takeRecords(): IntersectionObserverEntry[] {
                return [];
            }
            root = null;
            rootMargin = "";
            thresholds = [];
        }
        (window as unknown as { IntersectionObserver: typeof IntersectionObserverStub }).IntersectionObserver =
            IntersectionObserverStub;
    }
}
