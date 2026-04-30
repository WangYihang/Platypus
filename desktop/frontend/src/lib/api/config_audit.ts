// Frontend client for the per-host configuration-audit endpoints.
// Mirrors the security-scan client in hosts.ts (intentionally similar
// shape so the UI can reuse most of its rendering helpers), but the
// vocabulary is distinct: `risk` instead of `severity`, `leak`
// instead of `finding`, `auditor` instead of `check`. That avoids the
// two domains accidentally cross-pollinating in either direction.

import { authJSON } from "../auth";

export type Risk = "high" | "medium" | "low" | "info";

export interface RiskCounts {
    high: number;
    medium: number;
    low: number;
    info: number;
}

export interface ConfigLeak {
    id: string;
    leak_id: string;
    auditor_id: string;
    category: string;
    risk: Risk;
    title: string;
    location: string;
    /**
     * Already-redacted snippet (e.g. "AKIA****WXYZ"). Plaintext
     * credentials are never put on the wire — the agent applies
     * RedactSecret before transmission and the server stores only
     * the redacted form. The UI applies one extra defensive check
     * via the <MatchCell> renderer.
     */
    match: string;
    pattern: string;
    description: string;
    remediation: string;
    references?: string[];
    // Present only on the project-level (cross-host) endpoint, not
    // the per-host audit response — kept here so a single ConfigLeak
    // type covers both call sites.
    host_id?: string;
    scanned_at_unix?: number;
}

export interface AuditorResult {
    id: string;
    category: string;
    status: "ok" | "skipped" | "error";
    error?: string;
    elapsed_ms: number;
    leak_count: number;
}

export interface HostConfigAudit {
    id: string;
    host_id: string;
    project_id: string;
    started_at_unix: number;
    elapsed_ms: number;
    error?: string;
    risk_counts: RiskCounts;
    leaks: ConfigLeak[];
    auditors: AuditorResult[];
}

export interface ConfigAuditSummary {
    id: string;
    started_at_unix: number;
    elapsed_ms: number;
    risk_counts: RiskCounts;
    error?: string;
}

export interface ReauditHostOpts {
    auditor_ids?: string[];
    categories?: string[];
    per_auditor_timeout_ms?: number;
}

// AvailableAuditor mirrors the v2pb.AvailableAuditor proto. The UI's
// auditor checklist + Coverage panel renders these before any audit
// has run, so first-time visitors see what the agent *would* inspect.
// `applicable` reflects the agent's Applicable() decision at
// enumeration time — non-applicable rows render dimmed.
export interface AvailableAuditor {
    id: string;
    category: string;
    applicable: boolean;
    title?: string;
    description?: string;
    references?: string[];
}

// listAvailableAuditors proxies the agent's ListConfigAuditors RPC.
// Returns null when the agent is offline (404) so the UI can fall
// back to deriving the list from the persisted audit's auditors[].
export async function listAvailableAuditors(
    pid: string,
    hid: string,
): Promise<AvailableAuditor[] | null> {
    try {
        const j = await authJSON<{ auditors: AvailableAuditor[] }>(
            `/api/v1/projects/${pid}/hosts/${hid}/config-auditors`,
        );
        return j.auditors ?? [];
    } catch (err) {
        if (err instanceof Error && err.message.startsWith("404:")) return null;
        throw err;
    }
}

// getHostConfigAudit returns the latest persisted audit, or null when
// the host has never been audited. The null pathway is meaningful:
// the UI must distinguish never-audited from audited-clean.
export async function getHostConfigAudit(
    pid: string,
    hid: string,
    auditID?: string,
): Promise<HostConfigAudit | null> {
    const qs = auditID ? `?audit_id=${encodeURIComponent(auditID)}` : "";
    try {
        return await authJSON<HostConfigAudit>(
            `/api/v1/projects/${pid}/hosts/${hid}/config-audit${qs}`,
        );
    } catch (err) {
        if (err instanceof Error && err.message.startsWith("404:")) return null;
        throw err;
    }
}

// reauditHost POSTs to the same path. Triggers a fresh agent audit,
// persists the result server-side, and returns it. Throws on 4xx/5xx
// — callers surface humanizeError(err) inline rather than clobbering
// the cached read.
export async function reauditHost(
    pid: string,
    hid: string,
    opts: ReauditHostOpts = {},
): Promise<HostConfigAudit> {
    return authJSON<HostConfigAudit>(
        `/api/v1/projects/${pid}/hosts/${hid}/config-audit`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(opts),
        },
    );
}

// listHostAudits returns the lightweight per-host history rows for
// the History dropdown. limit defaults to 10, server caps at 50.
export async function listHostAudits(
    pid: string,
    hid: string,
    limit?: number,
): Promise<ConfigAuditSummary[]> {
    const qs = limit && limit > 0 ? `?limit=${limit}` : "";
    const j = await authJSON<{ audits: ConfigAuditSummary[] }>(
        `/api/v1/projects/${pid}/hosts/${hid}/config-audits${qs}`,
    );
    return j.audits ?? [];
}
