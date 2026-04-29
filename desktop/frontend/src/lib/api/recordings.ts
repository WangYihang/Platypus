import { authFetch, authJSON } from "../auth";

// Mirrors handler_recordings.go. Every interactive shell opened via
// the v2 terminal endpoint is captured to an asciinema v2 cast file;
// the row metadata flows through here so RecordingsPage can list,
// preview, and manage them.

export type RecordingStatus = "recording" | "completed" | "failed";

export interface TerminalRecording {
    id: string;
    project_id: string;
    host_id: string;
    host_alias?: string;
    host_hostname?: string;
    agent_id?: string;
    user_id?: string;
    username?: string;
    cols: number;
    rows: number;
    shell?: string;
    title?: string;
    size_bytes: number;
    duration_ms: number;
    frame_count: number;
    status: RecordingStatus;
    error_message?: string;
    started_at: string;
    ended_at?: string;
}

export interface ListRecordingsOpts {
    cursor?: string;
    limit?: number;
    hostId?: string;
    userId?: string;
    agentId?: string;
    status?: RecordingStatus | "";
    q?: string;
}

export interface ListRecordingsResponse {
    items: TerminalRecording[];
    total: number;
    next_cursor?: string;
}

export async function listRecordings(
    pid: string,
    opts: ListRecordingsOpts = {},
): Promise<ListRecordingsResponse> {
    const p = new URLSearchParams();
    if (opts.cursor) p.set("cursor", opts.cursor);
    if (opts.limit) p.set("limit", String(opts.limit));
    if (opts.hostId) p.set("host_id", opts.hostId);
    if (opts.userId) p.set("user_id", opts.userId);
    if (opts.agentId) p.set("agent_id", opts.agentId);
    if (opts.status) p.set("status", opts.status);
    if (opts.q) p.set("q", opts.q);
    const qs = p.toString();
    const path = `/api/v1/projects/${pid}/recordings${qs ? "?" + qs : ""}`;
    return authJSON<ListRecordingsResponse>(path);
}

export async function getRecording(pid: string, id: string): Promise<TerminalRecording> {
    return authJSON<TerminalRecording>(`/api/v1/projects/${pid}/recordings/${id}`);
}

// asciinema-player's fetch loader can't carry a Bearer header, so we
// pull the .cast bytes via authFetch and hand the player a blob URL.
export async function fetchRecordingCastBlob(pid: string, id: string): Promise<Blob> {
    const r = await authFetch(`/api/v1/projects/${pid}/recordings/${id}/cast`);
    return r.blob();
}

export async function updateRecording(
    pid: string,
    id: string,
    patch: { title?: string },
): Promise<TerminalRecording> {
    return authJSON<TerminalRecording>(`/api/v1/projects/${pid}/recordings/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
    });
}

export async function deleteRecording(pid: string, id: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/recordings/${id}`, { method: "DELETE" });
}
