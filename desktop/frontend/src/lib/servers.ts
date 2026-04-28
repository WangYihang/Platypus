// Server profile store — one "workspace" per Platypus server the user
// has saved. The sidebar's ServerSwitcher reads from here; the auth pool
// keys each session by `ServerProfile.id`; the notify pool opens a
// dedicated WebSocket per profile.
//
// Persistence: localStorage only. Refresh tokens live next door in
// auth.ts under a namespaced key; profiles here carry no secrets,
// just metadata the rail needs to render.
//
// Implementation: zustand store underlies the `profiles` + `activeId`
// state slots; the function-export API (listServers / addServer /
// onServersChange / …) is preserved so every existing consumer keeps
// working without an import-list change. The previous hand-rolled
// `Set<listener> + emit()` registry is gone; `onServersChange` now
// delegates to zustand's `subscribe`. Listener errors stay non-fatal
// for siblings (the wrapper below catches and logs).

import { create } from "zustand";

import { palette } from "../layout/theme";

export interface ServerProfile {
    id: string;
    name: string;
    url: string;
    order: number;
    createdAt: number;
}

const LS_SERVERS = "platypus.servers";
const LS_ACTIVE = "platypus.active_server";

// Legacy single-session keys from the pre-rail world. First boot in
// the new code reads them, synthesises one ServerProfile, and deletes
// them. Idempotent — subsequent boots see no legacy keys.
const LEGACY_LS_SERVER_URL = "platypus.server_url";

// Hard cap so localStorage doesn't bloat and the notify pool stays
// under browser per-origin WebSocket limits. 16 is comfortably more
// than any real user needs.
export const MAX_SERVERS = 16;

// --- Pure helpers (URL / hostname / id / avatar) ------------------------

export function normaliseURL(url: string): string {
    return url.replace(/\/+$/, "").trim();
}

// hostnameFromURL extracts a friendly default label from a URL; used
// when the user doesn't provide a name. Falls back to the raw string
// if URL parsing fails so we still produce *something* non-empty.
export function hostnameFromURL(url: string): string {
    try {
        return new URL(url).host || url;
    } catch {
        return url;
    }
}

// uid generates a short, unique id for profiles. We don't need real
// UUID rigor here — collision probability at 16 profiles is zero —
// but prefer crypto.randomUUID when available for consistency with
// the rest of the codebase.
function uid(): string {
    if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
        return crypto.randomUUID();
    }
    return `srv-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

// --- localStorage IO --------------------------------------------------

function readListFromStorage(): ServerProfile[] {
    try {
        const raw = localStorage.getItem(LS_SERVERS);
        if (!raw) return [];
        const parsed = JSON.parse(raw) as unknown;
        if (!Array.isArray(parsed)) return [];
        const out: ServerProfile[] = [];
        for (const entry of parsed) {
            if (!entry || typeof entry !== "object") continue;
            const e = entry as Partial<ServerProfile>;
            if (!e.id || !e.name || !e.url) continue;
            out.push({
                id: String(e.id),
                name: String(e.name),
                url: normaliseURL(String(e.url)),
                order: typeof e.order === "number" ? e.order : out.length,
                createdAt:
                    typeof e.createdAt === "number" ? e.createdAt : Date.now(),
            });
        }
        out.sort((a, b) => a.order - b.order);
        return out;
    } catch {
        return [];
    }
}

function writeListToStorage(list: ServerProfile[]): void {
    // Re-pack `order` so gaps from removals don't accumulate. Mutates
    // in place — only the persisted shape is observed.
    list.forEach((p, i) => {
        p.order = i;
    });
    try {
        localStorage.setItem(LS_SERVERS, JSON.stringify(list));
    } catch {
        // ignore quota / private mode
    }
}

function readActiveFromStorage(): string | null {
    try {
        return localStorage.getItem(LS_ACTIVE);
    } catch {
        return null;
    }
}

function writeActiveToStorage(id: string | null): void {
    try {
        if (id) localStorage.setItem(LS_ACTIVE, id);
        else localStorage.removeItem(LS_ACTIVE);
    } catch {
        // ignore
    }
}

// migrateLegacy lifts the old single-session localStorage keys into a
// single ServerProfile the first time the new code runs. Runs once at
// module load.
function migrateLegacyOnce(): {
    profiles: ServerProfile[];
    activeId: string | null;
} {
    const profiles = readListFromStorage();
    const activeId = readActiveFromStorage();
    if (profiles.length > 0) {
        // Already migrated — drop any leftover legacy breadcrumbs.
        try {
            localStorage.removeItem(LEGACY_LS_SERVER_URL);
        } catch {
            // ignore
        }
        return { profiles, activeId };
    }
    let legacyURL: string | null = null;
    try {
        legacyURL = localStorage.getItem(LEGACY_LS_SERVER_URL);
    } catch {
        // ignore
    }
    if (!legacyURL) return { profiles, activeId };

    const profile: ServerProfile = {
        id: uid(),
        name: hostnameFromURL(legacyURL),
        url: normaliseURL(legacyURL),
        order: 0,
        createdAt: Date.now(),
    };
    const seeded = [profile];
    writeListToStorage(seeded);
    writeActiveToStorage(profile.id);
    try {
        localStorage.removeItem(LEGACY_LS_SERVER_URL);
    } catch {
        // ignore
    }
    // Keep the legacy refresh token around — auth.ts picks it up
    // during its own migration pass so the user stays logged in.
    return { profiles: seeded, activeId: profile.id };
}

// --- Zustand store ---------------------------------------------------

interface ServersState {
    profiles: ServerProfile[];
    activeId: string | null;
}

const initial = migrateLegacyOnce();

export const useServersStore = create<ServersState>(() => ({
    profiles: initial.profiles,
    activeId: initial.activeId,
}));

// onServersChange wraps zustand's subscribe with the listener-error
// firewall we relied on previously. A throwing listener stays
// non-fatal for siblings.
export function onServersChange(fn: () => void): () => void {
    const wrapped = () => {
        try {
            fn();
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("servers listener threw:", err);
        }
    };
    return useServersStore.subscribe(wrapped);
}

// --- Function-export API ---------------------------------------------

export function listServers(): ServerProfile[] {
    return useServersStore.getState().profiles;
}

export function getServer(id: string): ServerProfile | null {
    return useServersStore.getState().profiles.find((s) => s.id === id) ?? null;
}

export interface AddServerInput {
    name?: string;
    url: string;
}

export class TooManyServersError extends Error {
    constructor() {
        super(`Reached ${MAX_SERVERS}-server limit`);
    }
}

export function addServer(input: AddServerInput): ServerProfile {
    const { profiles } = useServersStore.getState();
    if (profiles.length >= MAX_SERVERS) {
        throw new TooManyServersError();
    }
    const url = normaliseURL(input.url);
    const profile: ServerProfile = {
        id: uid(),
        name: input.name?.trim() || hostnameFromURL(url),
        url,
        order: profiles.length,
        createdAt: Date.now(),
    };
    const next = [...profiles, profile];
    writeListToStorage(next);
    useServersStore.setState({ profiles: next });
    return profile;
}

export function renameServer(id: string, name: string): void {
    const { profiles } = useServersStore.getState();
    const idx = profiles.findIndex((s) => s.id === id);
    if (idx < 0) return;
    const trimmed = name.trim() || profiles[idx].name;
    if (trimmed === profiles[idx].name) return;
    const next = profiles.slice();
    next[idx] = { ...profiles[idx], name: trimmed };
    writeListToStorage(next);
    useServersStore.setState({ profiles: next });
}

export function removeServer(id: string): void {
    const { profiles, activeId } = useServersStore.getState();
    const next = profiles.filter((s) => s.id !== id);
    writeListToStorage(next);
    // Active pointer follows the rail: if the active one is gone,
    // fall back to the first remaining or clear.
    const nextActive = activeId === id ? (next[0]?.id ?? null) : activeId;
    if (nextActive !== activeId) writeActiveToStorage(nextActive);
    useServersStore.setState({ profiles: next, activeId: nextActive });
}

export function reorderServers(orderedIds: string[]): void {
    const { profiles } = useServersStore.getState();
    const byId = new Map(profiles.map((s) => [s.id, s]));
    const next: ServerProfile[] = [];
    for (const id of orderedIds) {
        const s = byId.get(id);
        if (s) next.push(s);
    }
    // Tail anything the caller forgot so we don't drop profiles on a
    // partial reorder.
    for (const s of profiles) {
        if (!orderedIds.includes(s.id)) next.push(s);
    }
    writeListToStorage(next);
    useServersStore.setState({ profiles: next });
}

export function getActiveServerId(): string | null {
    return useServersStore.getState().activeId;
}

export function setActiveServerId(id: string | null): void {
    writeActiveToStorage(id);
    if (useServersStore.getState().activeId === id) return;
    useServersStore.setState({ activeId: id });
}

export function getActiveServer(): ServerProfile | null {
    const { profiles, activeId } = useServersStore.getState();
    if (!activeId) return null;
    return profiles.find((s) => s.id === activeId) ?? null;
}

// --- Avatar helpers ---------------------------------------------------

// avatarBg returns a stable palette colour for a URL so the same
// server always renders in the same hue across reloads and machines.
// djb2 keeps the implementation trivial; collisions across just ten
// buckets are acceptable here (the letter disambiguates).
export function avatarBg(url: string): string {
    const bgs = palette.avatarBgs;
    let hash = 5381;
    for (let i = 0; i < url.length; i++) {
        hash = ((hash << 5) + hash + url.charCodeAt(i)) | 0;
    }
    return bgs[Math.abs(hash) % bgs.length];
}

export interface Avatar {
    letter: string;
    bg: string;
    fg: string;
}

export function avatarFor(profile: ServerProfile): Avatar {
    const letter = profile.name.trim().charAt(0).toUpperCase() || "?";
    return { letter, bg: avatarBg(profile.url), fg: "#ffffff" };
}
