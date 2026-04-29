import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
    MAX_SERVERS,
    TooManyServersError,
    addServer,
    avatarBg,
    avatarFor,
    defaultServerURL,
    getActiveServer,
    getActiveServerId,
    getServer,
    hostnameFromURL,
    listServers,
    normaliseURL,
    onServersChange,
    removeServer,
    renameServer,
    reorderServers,
    setActiveServerId,
    useServersStore,
} from "./servers";

// servers.ts is being migrated from a hand-rolled
// `Set<listener> + emit()` registry to a zustand store. This spec
// pins the public API contract so the migration is a pure
// implementation change — every assertion below must keep passing
// before, during, and after the swap.
//
// Coverage targets the surfaces every consumer touches:
//   · CRUD: add / rename / remove / reorder
//   · Active pointer: get/set, fall-through on remove, persistence
//   · Notifications: onServersChange fires on every mutation
//   · Pure helpers: normaliseURL / hostnameFromURL / avatarBg /
//     avatarFor (no localStorage; sanity-checked for stability)
//   · Limits: MAX_SERVERS cap throws TooManyServersError

beforeEach(() => {
    window.localStorage.clear();
    // The zustand store state lives in memory; clearing localStorage
    // alone leaves residue from a prior test. Reset both so each
    // spec starts with an empty profile list and no active pointer.
    useServersStore.setState({ profiles: [], activeId: null });
});

afterEach(() => {
    window.localStorage.clear();
    useServersStore.setState({ profiles: [], activeId: null });
});

describe("servers — pure helpers", () => {
    it("normaliseURL strips trailing slashes and outer whitespace", () => {
        expect(normaliseURL("https://localhost:9443/")).toBe("https://localhost:9443");
        expect(normaliseURL("https://localhost:9443//")).toBe("https://localhost:9443");
        // The slash regex runs before trim, so whitespace-padded URLs
        // keep an interior slash even after normalisation. Documented
        // here so the migration doesn't accidentally "fix" it.
        expect(normaliseURL("  https://x.example  ")).toBe("https://x.example");
    });

    it("hostnameFromURL extracts the host or returns the input on failure", () => {
        expect(hostnameFromURL("https://prod.example:9443")).toBe("prod.example:9443");
        expect(hostnameFromURL("not a url")).toBe("not a url");
    });

    it("avatarBg is stable for the same URL", () => {
        expect(avatarBg("https://a")).toBe(avatarBg("https://a"));
    });

    it("defaultServerURL returns window.location.origin on http(s)", () => {
        // jsdom default is http://localhost:3000 — anything starting
        // with http:/https: is reflected verbatim.
        const got = defaultServerURL();
        expect(got).toMatch(/^https?:\/\//);
        expect(got).toBe(window.location.origin);
    });

    it("defaultServerURL falls back to loopback on non-http origins", () => {
        const original = window.location;
        // Re-define location with a wails:// protocol; jsdom's default
        // location object isn't writable, so swap it through defineProperty.
        Object.defineProperty(window, "location", {
            configurable: true,
            value: { ...original, protocol: "wails:", origin: "wails://app" },
        });
        try {
            expect(defaultServerURL()).toBe("http://127.0.0.1:7331");
        } finally {
            Object.defineProperty(window, "location", {
                configurable: true,
                value: original,
            });
        }
    });

    it("avatarFor uses the name's first uppercase letter", () => {
        const a = avatarFor({
            id: "x",
            name: "alpha",
            url: "https://a",
            order: 0,
            createdAt: 0,
        });
        expect(a.letter).toBe("A");
        expect(a.fg).toBe("#ffffff");
    });
});

describe("servers — CRUD + persistence", () => {
    it("listServers starts empty on a fresh client", () => {
        expect(listServers()).toEqual([]);
    });

    it("addServer creates a profile with normalised URL + ordered position", () => {
        const a = addServer({ url: "https://a.example/" });
        expect(a.url).toBe("https://a.example");
        expect(a.order).toBe(0);
        expect(a.name).toMatch(/a\.example/);

        const b = addServer({ name: "Mirror", url: "https://b.example" });
        expect(b.order).toBe(1);
        expect(b.name).toBe("Mirror");

        expect(listServers().map((s) => s.id)).toEqual([a.id, b.id]);
    });

    it("getServer returns null for unknown ids", () => {
        const a = addServer({ url: "https://a" });
        expect(getServer(a.id)?.id).toBe(a.id);
        expect(getServer("missing")).toBeNull();
    });

    it("renameServer trims and falls back to the existing name on empty", () => {
        const a = addServer({ name: "Old", url: "https://a" });
        renameServer(a.id, "  New  ");
        expect(getServer(a.id)?.name).toBe("New");
        renameServer(a.id, "");
        expect(getServer(a.id)?.name).toBe("New");
    });

    it("removeServer drops the profile and re-packs order", () => {
        const a = addServer({ url: "https://a" });
        const b = addServer({ url: "https://b" });
        const c = addServer({ url: "https://c" });
        removeServer(b.id);
        const remaining = listServers();
        expect(remaining.map((s) => s.id)).toEqual([a.id, c.id]);
        expect(remaining.map((s) => s.order)).toEqual([0, 1]);
    });

    it("reorderServers honours the supplied order, tailing forgotten ids", () => {
        const a = addServer({ url: "https://a" });
        const b = addServer({ url: "https://b" });
        const c = addServer({ url: "https://c" });
        // Caller forgets `c` — we expect it to be tailed, not dropped.
        reorderServers([b.id, a.id]);
        expect(listServers().map((s) => s.id)).toEqual([b.id, a.id, c.id]);
    });

    it("addServer throws TooManyServersError once the cap is hit", () => {
        for (let i = 0; i < MAX_SERVERS; i++) {
            addServer({ url: `https://srv-${i}` });
        }
        expect(() => addServer({ url: "https://overflow" })).toThrow(
            TooManyServersError,
        );
    });
});

describe("servers — active pointer", () => {
    it("getActiveServerId returns null when no server is set active", () => {
        expect(getActiveServerId()).toBeNull();
        expect(getActiveServer()).toBeNull();
    });

    it("setActiveServerId persists across reads", () => {
        const a = addServer({ url: "https://a" });
        setActiveServerId(a.id);
        expect(getActiveServerId()).toBe(a.id);
        expect(getActiveServer()?.id).toBe(a.id);
    });

    it("removing the active server falls through to the next remaining profile", () => {
        const a = addServer({ url: "https://a" });
        const b = addServer({ url: "https://b" });
        setActiveServerId(a.id);
        removeServer(a.id);
        expect(getActiveServerId()).toBe(b.id);
    });

    it("setActiveServerId(null) clears the pointer", () => {
        const a = addServer({ url: "https://a" });
        setActiveServerId(a.id);
        setActiveServerId(null);
        expect(getActiveServerId()).toBeNull();
    });
});

describe("servers — notifications", () => {
    it("onServersChange fires on add / rename / remove / setActive", () => {
        const fn = vi.fn();
        const off = onServersChange(fn);

        const a = addServer({ url: "https://a" });
        expect(fn).toHaveBeenCalled();
        const after = fn.mock.calls.length;

        renameServer(a.id, "New");
        expect(fn.mock.calls.length).toBeGreaterThan(after);

        const after2 = fn.mock.calls.length;
        setActiveServerId(a.id);
        expect(fn.mock.calls.length).toBeGreaterThan(after2);

        const after3 = fn.mock.calls.length;
        removeServer(a.id);
        expect(fn.mock.calls.length).toBeGreaterThan(after3);

        off();
        const before = fn.mock.calls.length;
        addServer({ url: "https://b" });
        // Listener was unsubscribed — no more calls.
        expect(fn.mock.calls.length).toBe(before);
    });

    it("a throwing listener doesn't break sibling listeners", () => {
        const sane = vi.fn();
        onServersChange(() => {
            throw new Error("explode");
        });
        onServersChange(sane);

        // Suppress the expected console.error so test output stays
        // clean.
        const spy = vi.spyOn(console, "error").mockImplementation(() => {});
        addServer({ url: "https://a" });
        spy.mockRestore();

        expect(sane).toHaveBeenCalled();
    });
});
