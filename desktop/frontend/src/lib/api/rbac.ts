import { authFetch, authJSON } from "../auth";

// Wired off /api/v1/admin/{permissions,roles}. Every route is gated
// server-side by RequireGlobalRole(admin); the UI hides the page from
// non-admins via getSessionUser().role.

export interface RBACPermission {
    slug: string;
    resource: string;
    description: string;
}

export interface RBACRoleSummary {
    slug: string;
    name: string;
    description?: string;
    is_builtin: boolean;
    is_global: boolean;
    is_project: boolean;
    created_at: string;
    updated_at: string;
}

export interface RBACRole extends RBACRoleSummary {
    permissions: string[];
}

export interface CreateRBACRoleRequest {
    slug: string;
    name: string;
    description?: string;
    is_global: boolean;
    is_project: boolean;
    permissions: string[];
}

export interface UpdateRBACRoleRequest {
    name?: string;
    description?: string;
    is_global?: boolean;
    is_project?: boolean;
    permissions?: string[];
}

export async function listRBACPermissions(): Promise<RBACPermission[]> {
    const j = await authJSON<{ permissions: RBACPermission[] }>(`/api/v1/admin/permissions`);
    return j.permissions ?? [];
}

export async function listRBACRoles(opts?: {
    isGlobal?: boolean;
    isProject?: boolean;
}): Promise<RBACRoleSummary[]> {
    const params = new URLSearchParams();
    if (opts?.isGlobal !== undefined) params.set("is_global", String(opts.isGlobal));
    if (opts?.isProject !== undefined) params.set("is_project", String(opts.isProject));
    const q = params.toString();
    const j = await authJSON<{ roles: RBACRoleSummary[] }>(
        `/api/v1/admin/roles${q ? `?${q}` : ""}`,
    );
    return j.roles ?? [];
}

export async function getRBACRole(slug: string): Promise<RBACRole> {
    return authJSON<RBACRole>(`/api/v1/admin/roles/${encodeURIComponent(slug)}`);
}

export async function createRBACRole(req: CreateRBACRoleRequest): Promise<RBACRole> {
    return authJSON<RBACRole>(`/api/v1/admin/roles`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
    });
}

export async function updateRBACRole(
    slug: string,
    req: UpdateRBACRoleRequest,
): Promise<RBACRole> {
    return authJSON<RBACRole>(`/api/v1/admin/roles/${encodeURIComponent(slug)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
    });
}

export async function deleteRBACRole(slug: string): Promise<void> {
    const r = await authFetch(`/api/v1/admin/roles/${encodeURIComponent(slug)}`, {
        method: "DELETE",
    });
    if (!r.ok && r.status !== 404) throw new Error(`${r.status}: ${await r.text()}`);
}
