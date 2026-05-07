import { authFetch, authJSON } from "../auth";

// Project-scoped, AES-256-GCM-encrypted secret store. Plugin
// configs reference these by id (see PluginSpec.config_overrides
// — sensitive fields carry a {"$secret":"sec_<id>"} placeholder
// that the server resolves at install time).
//
// There is deliberately no "reveal a secret over HTTP" API. Once
// the value is sealed under the project KEK on Create, every
// subsequent path returns the redacted view; the only place the
// plaintext lives is inside the install pipeline's resolver.

export interface ProjectSecretRedacted {
    secret_id: string;
    project_id: string;
    name: string;
    description?: string;
    created_by_user?: string;
    created_at: string;
    last_used_at?: string;
    revoked: boolean;
    revoked_at?: string;
}

export interface CreateProjectSecretRequest {
    name: string;
    description?: string;
    // Plaintext supplied once at create time. The FE should clear
    // its copy as soon as the request resolves.
    value: string;
}

export async function listProjectSecrets(
    pid: string,
): Promise<ProjectSecretRedacted[]> {
    const j = await authJSON<{ secrets: ProjectSecretRedacted[] }>(
        `/api/v1/projects/${pid}/secrets`,
    );
    return j.secrets ?? [];
}

export async function createProjectSecret(
    pid: string,
    body: CreateProjectSecretRequest,
): Promise<ProjectSecretRedacted> {
    return authJSON<ProjectSecretRedacted>(
        `/api/v1/projects/${pid}/secrets`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        },
    );
}

export async function deleteProjectSecret(
    pid: string,
    secretID: string,
    reason?: string,
): Promise<void> {
    const q = reason ? `?reason=${encodeURIComponent(reason)}` : "";
    const r = await authFetch(
        `/api/v1/projects/${pid}/secrets/${secretID}${q}`,
        { method: "DELETE" },
    );
    if (!r.ok && r.status !== 404) {
        throw new Error(`${r.status}: ${await r.text()}`);
    }
}
