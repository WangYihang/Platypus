import { authFetch, authJSON } from "../auth";

export interface UserRow {
    id: string;
    username: string;
    // Free-form slug after RBAC: builtin (admin / operator / viewer)
    // plus any custom role from /admin/access-control.
    role: string;
}

export async function listUsers(): Promise<UserRow[]> {
    const j = await authJSON<{ users: UserRow[] }>("/api/v1/users");
    return j.users;
}

export async function createUser(
    username: string,
    password: string,
    role: string,
): Promise<UserRow> {
    return authJSON<UserRow>("/api/v1/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, role }),
    });
}

export async function updateUser(
    id: string,
    patch: { role?: string; password?: string },
): Promise<UserRow> {
    return authJSON<UserRow>(`/api/v1/users/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
    });
}

export async function deleteUser(id: string): Promise<void> {
    await authFetch(`/api/v1/users/${id}`, { method: "DELETE" });
}
