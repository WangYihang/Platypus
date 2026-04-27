import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
    preferenceDefaults,
    readPreference,
    resetPreference,
    writePreference,
} from "./preferences";

// preferences.ts is a localStorage-backed registry of client-local
// settings. These tests pin the contract every consumer relies on:
//   · readPreference returns the default if no value has been written
//   · writePreference round-trips through localStorage
//   · resetPreference removes the key so subsequent reads see the default
//   · malformed JSON in localStorage falls back to the default
// They also serve as the smoke check that the vitest + jsdom harness
// is wired up — if these fail, every other test in this repo is too.

describe("preferences", () => {
    beforeEach(() => {
        localStorage.clear();
    });

    afterEach(() => {
        localStorage.clear();
    });

    it("returns the registered default when nothing is stored", () => {
        expect(readPreference("ui.density")).toBe(preferenceDefaults["ui.density"]);
        expect(readPreference("terminal.fontSize")).toBe(
            preferenceDefaults["terminal.fontSize"],
        );
    });

    it("round-trips a string preference through localStorage", () => {
        writePreference("ui.density", "compact");
        expect(readPreference("ui.density")).toBe("compact");
    });

    it("round-trips a numeric preference", () => {
        writePreference("terminal.fontSize", 18);
        expect(readPreference("terminal.fontSize")).toBe(18);
    });

    it("round-trips a boolean preference", () => {
        writePreference("ui.confirmDelete", false);
        expect(readPreference("ui.confirmDelete")).toBe(false);
    });

    it("resetPreference deletes the stored key so reads see the default", () => {
        writePreference("ui.density", "compact");
        expect(readPreference("ui.density")).toBe("compact");
        resetPreference("ui.density");
        expect(readPreference("ui.density")).toBe(preferenceDefaults["ui.density"]);
    });

    it("falls back to the default when the stored JSON is malformed", () => {
        // Simulate a corrupt write — e.g. a different app version that
        // wrote a different shape to the same key.
        localStorage.setItem("platypus.pref.ui.density", "{not json");
        expect(readPreference("ui.density")).toBe(preferenceDefaults["ui.density"]);
    });
});
