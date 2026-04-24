// Server profile store — one "workspace" per Platypus server the user
// has saved. The Slack-style ServerRail reads from here; the auth pool
// keys each session by `ServerProfile.id`; the notify pool opens a
// dedicated WebSocket per profile.
//
// Persistence: localStorage only. Refresh tokens live next door in
// auth.ts under a namespaced key; profiles here carry no secrets,
// just metadata the rail needs to render.

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
const LEGACY_LS_REFRESH = "platypus.refresh_token";

// Hard cap so localStorage doesn't bloat and the notify pool stays
// under browser per-origin WebSocket limits. 16 is comfortably more
// than any real user needs.
export const MAX_SERVERS = 16;

const listeners = new Set<() => void>();

function emit() {
    for (const fn of listeners) {
        try {
            fn();
        } catch (err) {
            // eslint-disable-next-line no-console
            console.error("servers listener threw:", err);
        }
    }
}

export function onServersChange(fn: () => void): () => void {
    listeners.add(fn);
    return () => {
        listeners.delete(fn);
    };
}

function readList(): ServerProfile[] {
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
                createdAt: typeof e.createdAt === "number" ? e.createdAt : Date.now(),
            });
        }
        out.sort((a, b) => a.order - b.order);
        return out;
    } catch {
        return [];
    }
}

function writeList(list: ServerProfile[]): void {
    // Re-pack `order` so gaps from removals don't accumulate.
    list.forEach((p, i) => {
        p.order = i;
    });
    localStorage.setItem(LS_SERVERS, JSON.stringify(list));
    emit();
}

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

// migrateLegacy lifts the old single-session localStorage keys into a
// single ServerProfile the first time the new code runs. Called
// implicitly from listServers() so every entry point is covered.
let migrated = false;
function migrateLegacy(): void {
    if (migrated) return;
    migrated = true;
    try {
        const url = localStorage.getItem(LEGACY_LS_SERVER_URL);
        if (!url) return;
        const current = readListRaw();
        if (current.length > 0) {
            // Already migrated — drop the legacy breadcrumbs.
            localStorage.removeItem(LEGACY_LS_SERVER_URL);
            return;
        }
        const profile: ServerProfile = {
            id: uid(),
            name: hostnameFromURL(url),
            url: normaliseURL(url),
            order: 0,
            createdAt: Date.now(),
        };
        localStorage.setItem(LS_SERVERS, JSON.stringify([profile]));
        localStorage.setItem(LS_ACTIVE, profile.id);
        // Keep the legacy refresh token around — auth.ts picks it up
        // during its own migration pass so the user stays logged in.
        localStorage.removeItem(LEGACY_LS_SERVER_URL);
    } catch {
        // localStorage disabled; nothing to do.
    }
}

function readListRaw(): ServerProfile[] {
    try {
        const raw = localStorage.getItem(LS_SERVERS);
        if (!raw) return [];
        const parsed = JSON.parse(raw) as unknown;
        return Array.isArray(parsed) ? (parsed as ServerProfile[]) : [];
    } catch {
        return [];
    }
}

export function listServers(): ServerProfile[] {
    migrateLegacy();
    return readList();
}

export function getServer(id: string): ServerProfile | null {
    return listServers().find((s) => s.id === id) ?? null;
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
    const list = listServers();
    if (list.length >= MAX_SERVERS) {
        throw new TooManyServersError();
    }
    const url = normaliseURL(input.url);
    const profile: ServerProfile = {
        id: uid(),
        name: input.name?.trim() || hostnameFromURL(url),
        url,
        order: list.length,
        createdAt: Date.now(),
    };
    writeList([...list, profile]);
    return profile;
}

export function renameServer(id: string, name: string): void {
    const list = listServers();
    const idx = list.findIndex((s) => s.id === id);
    if (idx < 0) return;
    list[idx] = { ...list[idx], name: name.trim() || list[idx].name };
    writeList(list);
}

export function removeServer(id: string): void {
    const list = listServers().filter((s) => s.id !== id);
    writeList(list);
    // Active pointer follows the rail: if the active one is gone,
    // fall back to the first remaining or clear.
    if (getActiveServerId() === id) {
        setActiveServerId(list[0]?.id ?? null);
    }
}

export function reorderServers(orderedIds: string[]): void {
    const list = listServers();
    const byId = new Map(list.map((s) => [s.id, s]));
    const next: ServerProfile[] = [];
    for (const id of orderedIds) {
        const s = byId.get(id);
        if (s) next.push(s);
    }
    // Tail anything the caller forgot so we don't drop profiles on a
    // partial reorder.
    for (const s of list) {
        if (!orderedIds.includes(s.id)) next.push(s);
    }
    writeList(next);
}

export function getActiveServerId(): string | null {
    migrateLegacy();
    try {
        return localStorage.getItem(LS_ACTIVE);
    } catch {
        return null;
    }
}

export function setActiveServerId(id: string | null): void {
    try {
        if (id) {
            localStorage.setItem(LS_ACTIVE, id);
        } else {
            localStorage.removeItem(LS_ACTIVE);
        }
    } catch {
        // ignore
    }
    emit();
}

export function getActiveServer(): ServerProfile | null {
    const id = getActiveServerId();
    if (!id) return null;
    return getServer(id);
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
