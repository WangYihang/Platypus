import { authFetch, authJSON } from "../auth";

import type { PluginSpecRef } from "./install";

// Saved enrollment configurations. A preset captures the operator's
// wizard inputs (target OS/arch, TTL, PAT max-uses, baseline plugins,
// …) so repeat enrolments collapse to "pick preset → Generate" instead
// of walking the 11-step flow every time.
//
// Presets are NOT credentials — every preset use still mints a fresh
// single-use install token via /install-artifacts. This API is purely
// the saved-input-template surface.

export interface EnrollmentPreset {
    preset_id: string;
    project_id: string;
    name: string;
    description?: string;
    server_endpoint?: string;
    target_os?: string;
    target_arch?: string;
    ttl_seconds?: number;
    pat_max_uses?: number;
    auto_approve: boolean;
    skip_tls_verification: boolean;
    // plugin_specs is the rich shape: plugin_id + version +
    // granted_capabilities + config_overrides + schema_version per
    // entry. Server emits both this AND the legacy
    // baseline_plugin_ids during the migration window so consumers
    // that haven't been upgraded keep working. New code reads
    // plugin_specs.
    plugin_specs?: PluginSpecRef[];
    baseline_plugin_ids?: string[];
    pat_description?: string;
    // is_seed flags the three system-default presets seeded on first
    // wizard open of a fresh project. Operators can edit / delete them
    // freely — the flag drives the "system default" badge in the UI.
    is_seed: boolean;
    created_by_user?: string;
    created_at: string;
    updated_at: string;
}

export type UpsertEnrollmentPresetRequest = Omit<
    EnrollmentPreset,
    | "preset_id"
    | "project_id"
    | "is_seed"
    | "created_by_user"
    | "created_at"
    | "updated_at"
>;

export async function listEnrollmentPresets(
    pid: string,
): Promise<EnrollmentPreset[]> {
    const j = await authJSON<{ presets: EnrollmentPreset[] }>(
        `/api/v1/projects/${pid}/enrollment-presets`,
    );
    return j.presets ?? [];
}

// seedEnrollmentPresets is idempotent on the server — INSERT OR IGNORE
// against (project_id, name) — so the FE can call it any time the list
// comes back empty and trust that re-runs are no-ops.
export async function seedEnrollmentPresets(
    pid: string,
): Promise<EnrollmentPreset[]> {
    const j = await authJSON<{ presets: EnrollmentPreset[] }>(
        `/api/v1/projects/${pid}/enrollment-presets/seed`,
        { method: "POST" },
    );
    return j.presets ?? [];
}

export async function createEnrollmentPreset(
    pid: string,
    body: UpsertEnrollmentPresetRequest,
): Promise<EnrollmentPreset> {
    return authJSON<EnrollmentPreset>(
        `/api/v1/projects/${pid}/enrollment-presets`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        },
    );
}

export async function updateEnrollmentPreset(
    pid: string,
    presetID: string,
    body: UpsertEnrollmentPresetRequest,
): Promise<EnrollmentPreset> {
    return authJSON<EnrollmentPreset>(
        `/api/v1/projects/${pid}/enrollment-presets/${presetID}`,
        {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        },
    );
}

export async function deleteEnrollmentPreset(
    pid: string,
    presetID: string,
): Promise<void> {
    const r = await authFetch(
        `/api/v1/projects/${pid}/enrollment-presets/${presetID}`,
        { method: "DELETE" },
    );
    if (!r.ok && r.status !== 404) {
        throw new Error(`${r.status}: ${await r.text()}`);
    }
}
