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
}
