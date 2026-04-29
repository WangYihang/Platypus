import { authFetch, authJSON } from "../auth";

// User-issued, long-lived API tokens for the /account page Tokens tab.
// Distinct from EnrollmentToken (`plt_*`) — those are admin-issued
// one-shot agent-bootstrap secrets. Account PATs are GitHub-style
// `pat_*` strings a logged-in user creates for API access.
//
// Plaintext only returns from POST; every list / get returns metadata.

export interface AccountPAT {
    token_id: string;
    name: string;
    description?: string;
    scopes: string[];
    created_at: string;
    expires_at: string;
    last_used_at?: string;
    last_used_ip?: string;
    revoked: boolean;
    revoked_at?: string;
}

export interface IssueAccountPATRequest {
    name: string;
    description?: string;
    // Omit to default to caller's role-derived ceiling.
    scopes?: string[];
    // Defaults to 90d; capped server-side at 1y.
    ttl_seconds?: number;
}

export interface IssueAccountPATResponse {
    token_id: string;
    token: string; // pat_<id>.<secret> — plaintext exposed exactly once
    name: string;
    scopes: string[];
    created_at: string;
    expires_at: string;
}

export async function listAccountPATs(includeRevoked = false): Promise<AccountPAT[]> {
    const q = includeRevoked ? "?include_revoked=true" : "";
    const j = await authJSON<{ tokens: AccountPAT[] }>(`/api/v1/account/pat${q}`);
    return j.tokens ?? [];
}

export async function issueAccountPAT(
    req: IssueAccountPATRequest,
): Promise<IssueAccountPATResponse> {
    return authJSON<IssueAccountPATResponse>(`/api/v1/account/pat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
    });
}

export async function revokeAccountPAT(tokenID: string): Promise<void> {
    const r = await authFetch(`/api/v1/account/pat/${tokenID}`, { method: "DELETE" });
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}

// Effective permission set the calling user holds — drives the PAT
// issue dialog's scope selector so checkboxes match what the server
// will accept.
export async function listMyPermissions(): Promise<string[]> {
    const j = await authJSON<{ permissions: string[] }>(`/api/v1/account/permissions`);
    return j.permissions ?? [];
}
