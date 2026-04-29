import { authFetch, authJSON } from "../auth";

// Runtime-tunable policy knobs (token TTLs, release channel, presigned
// TTL, mesh discovery) wired via GET/PUT/DELETE on
// /api/v1/admin/settings/:key.

export type SettingType =
    | "duration_seconds"
    | "bool"
    | "string"
    | "int"
    | "string_list";

// Mirrors internal/settings.SettingDescriptor. `effective` is the value
// the server is currently using; db / yaml hold the raw override /
// fallback values for the UI to show "source" hints.
export interface SettingDescriptor {
    key: string;
    type: SettingType;
    section: string;
    label: string;
    description: string;
    default: unknown;
    yaml?: unknown;
    db?: unknown;
    effective: unknown;
    source: "db" | "yaml" | "default";
}

export async function listSettings(): Promise<SettingDescriptor[]> {
    const j = await authJSON<{ settings: SettingDescriptor[] }>(
        "/api/v1/admin/settings",
    );
    return j.settings;
}

// Caller must convert form strings to the typed value (number / bool);
// the server re-validates against the registered type.
export async function updateSetting(
    key: string,
    value: unknown,
): Promise<void> {
    await authFetch(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value }),
    });
}

export async function resetSetting(key: string): Promise<void> {
    await authFetch(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
        method: "DELETE",
    });
}
