import { useCallback, useEffect, useState } from "react";

// preferences.ts is the localStorage-backed user-pref registry. Keep
// the keyspace flat and namespaced under "platypus.pref." so a future
// "wipe my settings" button can do a single prefix scan, and so the
// keys can't collide with the ad-hoc keys other modules already
// persist (servers, sessions, terminal drawer geometry, …).
//
// Every preference must:
//   · default to a sensible value (so a fresh install never feels
//     half-configured)
//   · be a serialisable shape (string, number, boolean, simple JSON)
//   · be readable + writable from any component via usePreference()
//
// Cross-tab updates: writes dispatch a `storage` event AND fire a
// local custom event so other hooks in the same tab re-read. Native
// `storage` events only fire across tabs, never within a single tab.

const PREFIX = "platypus.pref.";
const LOCAL_EVENT = "platypus.pref-change";

export interface PreferenceDefs {
    // --- Display
    "ui.density": "comfortable" | "compact";
    "ui.fleet.defaultView": "cards" | "table" | "timeline" | "graph";
    "ui.activities.defaultRange": "24h" | "7d" | "30d" | "all";
    "ui.files.viewMode": "list" | "grid";
    "ui.files.previewOpen": boolean;
    // When true, dotfiles (".git", ".bashrc", …) appear in the
    // listing. Default `false` to match the muscle-memory of `ls`,
    // Finder, and most file managers — the explorer's toolbar
    // exposes a one-click toggle when the user wants them back.
    "ui.files.showHidden": boolean;

    // --- Terminal
    "terminal.fontSize": number;
    "terminal.cursorBlink": boolean;
    "terminal.scrollback": number;

    // --- Behaviour
    "ui.confirmDelete": boolean;
}

const DEFAULTS: PreferenceDefs = {
    // Compact is the default at the *project level* — file lists,
    // host tables, and audit feeds all routinely show hundreds of
    // rows, and operators almost always want max density. The
    // Preferences UI lets a user flip to "comfortable" once.
    "ui.density": "compact",
    "ui.fleet.defaultView": "table",
    "ui.activities.defaultRange": "7d",
    "ui.files.viewMode": "list",
    "ui.files.previewOpen": true,
    "ui.files.showHidden": false,
    "terminal.fontSize": 13,
    "terminal.cursorBlink": true,
    "terminal.scrollback": 5000,
    "ui.confirmDelete": true,
};

function storageKey<K extends keyof PreferenceDefs>(k: K): string {
    return PREFIX + k;
}

function emitLocalChange(key: keyof PreferenceDefs) {
    try {
        window.dispatchEvent(
            new CustomEvent(LOCAL_EVENT, { detail: { key } }),
        );
    } catch {
        // SSR / non-DOM environment — nothing to notify.
    }
}

export function readPreference<K extends keyof PreferenceDefs>(
    key: K,
): PreferenceDefs[K] {
    try {
        const raw = localStorage.getItem(storageKey(key));
        if (raw === null) return DEFAULTS[key];
        return JSON.parse(raw) as PreferenceDefs[K];
    } catch {
        return DEFAULTS[key];
    }
}

export function writePreference<K extends keyof PreferenceDefs>(
    key: K,
    value: PreferenceDefs[K],
): void {
    try {
        localStorage.setItem(storageKey(key), JSON.stringify(value));
        emitLocalChange(key);
    } catch {
        // Quota / private mode — silently drop.
    }
}

export function resetPreference<K extends keyof PreferenceDefs>(key: K): void {
    try {
        localStorage.removeItem(storageKey(key));
        emitLocalChange(key);
    } catch {
        // ignore
    }
}

// usePreference is the React hook. The state slot is rehydrated from
// localStorage on first render and stays in sync with cross-tab
// `storage` events plus same-tab CustomEvents emitted by writePreference.
export function usePreference<K extends keyof PreferenceDefs>(
    key: K,
): [PreferenceDefs[K], (next: PreferenceDefs[K]) => void, () => void] {
    const [value, setValue] = useState<PreferenceDefs[K]>(() =>
        readPreference(key),
    );

    useEffect(() => {
        const onStorage = (e: StorageEvent) => {
            if (e.key === storageKey(key)) {
                setValue(readPreference(key));
            }
        };
        const onLocal = (e: Event) => {
            const ce = e as CustomEvent<{ key: string }>;
            if (ce.detail?.key === key) {
                setValue(readPreference(key));
            }
        };
        window.addEventListener("storage", onStorage);
        window.addEventListener(LOCAL_EVENT, onLocal as EventListener);
        return () => {
            window.removeEventListener("storage", onStorage);
            window.removeEventListener(LOCAL_EVENT, onLocal as EventListener);
        };
    }, [key]);

    const set = useCallback(
        (next: PreferenceDefs[K]) => {
            writePreference(key, next);
        },
        [key],
    );

    const reset = useCallback(() => {
        resetPreference(key);
    }, [key]);

    return [value, set, reset];
}

export const preferenceDefaults = DEFAULTS;
