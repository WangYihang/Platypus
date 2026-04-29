import { authFetch, authJSON } from "../auth";

// Single append-only `activities` table on the server backs both the
// live "what just happened" view and the compliance export. The legacy
// audit-export endpoint has been retired; use these entry points.

export type ActivityOutcome = "success" | "denied" | "error";

export interface ActivityItem {
    id: number;
    at: string;
    project_id: string | null;
    actor_type: string;
    actor_user: string;
    actor_ip?: string;
    actor_ua?: string;
    actor_token_id?: string;
    category: string;
    action: string;
    target_type?: string;
    target_id?: string;
    target_label?: string;
    outcome: ActivityOutcome;
    error?: string;
    duration_ms?: number;
    request_id?: string;
    session_id?: string;
    meta?: Record<string, unknown> | string;
}

// High-level "where did this row come from" alias the activities page
// exposes as a segment control. Each value maps to one or more raw
// actor_type values on the server (see expandSourceAlias in
// handler_activities_v1.go).
export type ActivitySource = "human" | "agent" | "system";

export interface ListActivitiesOpts {
    from?: Date;
    to?: Date;
    category?: string[];
    action?: string[];
    actor?: string;
    actorType?: string[];
    sources?: ActivitySource[];
    outcome?: ActivityOutcome;
    sessionId?: string;
    targetType?: string;
    targetId?: string;
    q?: string;
    limit?: number;
    cursor?: string;
    includeGlobal?: boolean;
    includeTotal?: boolean;
}

export interface ListActivitiesResponse {
    items: ActivityItem[];
    next_cursor?: string;
    total?: number;
}

// Centralised so list + export hit the same query shape the backend expects.
function buildActivityParams(opts: ListActivitiesOpts): URLSearchParams {
    const p = new URLSearchParams();
    if (opts.from) p.set("from", opts.from.toISOString());
    if (opts.to) p.set("to", opts.to.toISOString());
    if (opts.category && opts.category.length) p.set("category", opts.category.join(","));
    if (opts.action) {
        for (const a of opts.action) p.append("action", a);
    }
    if (opts.actor) p.set("actor", opts.actor);
    if (opts.actorType && opts.actorType.length) p.set("actor_type", opts.actorType.join(","));
    if (opts.sources && opts.sources.length) p.set("source", opts.sources.join(","));
    if (opts.outcome) p.set("outcome", opts.outcome);
    if (opts.sessionId) p.set("session_id", opts.sessionId);
    if (opts.targetType) p.set("target_type", opts.targetType);
    if (opts.targetId) p.set("target_id", opts.targetId);
    if (opts.q) p.set("q", opts.q);
    if (opts.limit) p.set("limit", String(opts.limit));
    if (opts.cursor) p.set("cursor", opts.cursor);
    if (opts.includeGlobal) p.set("include_global", "true");
    if (opts.includeTotal) p.set("include_total", "true");
    return p;
}

export async function listProjectActivities(
    pid: string,
    opts: ListActivitiesOpts = {},
): Promise<ListActivitiesResponse> {
    const qs = buildActivityParams(opts).toString();
    const path = `/api/v1/projects/${pid}/activities${qs ? "?" + qs : ""}`;
    return authJSON<ListActivitiesResponse>(path);
}

export async function listGlobalActivities(
    opts: ListActivitiesOpts = {},
): Promise<ListActivitiesResponse> {
    const qs = buildActivityParams(opts).toString();
    const path = `/api/v1/activities${qs ? "?" + qs : ""}`;
    return authJSON<ListActivitiesResponse>(path);
}

export async function exportProjectActivitiesBlob(
    pid: string,
    opts: ListActivitiesOpts & { format?: "jsonl" | "csv" } = {},
): Promise<Blob> {
    const p = buildActivityParams(opts);
    p.set("format", opts.format ?? "jsonl");
    const r = await authFetch(`/api/v1/projects/${pid}/activities/export?${p.toString()}`);
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
    return r.blob();
}
