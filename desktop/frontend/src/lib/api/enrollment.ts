import { authFetch, authJSON } from "../auth";

// Admin-only surface for provisioning enrollment tokens. Backend tables
// and URL segments still use the historical "PAT" name; the UI calls
// them enrollment tokens. They are NOT account-scoped API tokens —
// they're one-shot agent-bootstrap secrets that burn on first /enroll
// and are replaced by an mTLS cert.
//
// The plaintext `plt_*` only ever comes back in the response to POST;
// every list / get strips it.

export interface EnrollmentTokenListItem {
    token_id: string;
    description?: string;
    issued_by_user: string;
    issued_at: string;
    expires_at: string;
    max_uses: number;
    uses: number;
    binding_machine_id?: string;
    binding_host_alias?: string;
    revoked: boolean;
    revoked_at?: string;
    revoked_reason?: string;
    status: "pending" | "consumed" | "expired" | "revoked";
}

export interface IssueEnrollmentTokenResponse {
    token_id: string;
    token: string; // plt_<id>.<secret> — only time plaintext is exposed
    expires_at: string;
    issued_at: string;
    max_uses: number;
    description?: string;
}

export interface IssueEnrollmentTokenRequest {
    description?: string;
    ttl_seconds?: number;
    max_uses?: number;
    binding_machine_id?: string;
    binding_host_alias?: string;
}

export async function listEnrollmentTokens(
    pid: string,
    includeInactive = false,
): Promise<EnrollmentTokenListItem[]> {
    const q = includeInactive ? "?include_inactive=true" : "";
    const j = await authJSON<{ tokens: EnrollmentTokenListItem[] }>(
        `/api/v1/projects/${pid}/pat-tokens${q}`,
    );
    return j.tokens ?? [];
}

export async function issueEnrollmentToken(
    pid: string,
    req: IssueEnrollmentTokenRequest,
): Promise<IssueEnrollmentTokenResponse> {
    return authJSON<IssueEnrollmentTokenResponse>(
        `/api/v1/projects/${pid}/pat-tokens`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(req),
        },
    );
}

export async function revokeEnrollmentToken(
    pid: string,
    tokenID: string,
    reason?: string,
): Promise<void> {
    const q = reason ? `?reason=${encodeURIComponent(reason)}` : "";
    const r = await authFetch(
        `/api/v1/projects/${pid}/pat-tokens/${tokenID}${q}`,
        { method: "DELETE" },
    );
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}
