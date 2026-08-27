import { authFetch, authJSON } from "../auth";

export interface Project {
    id: string;
    name: string;
    slug: string;
    created_at: string;
    created_by: string;
    /**
     * Per-project opt-in for sending finished terminal recordings to an
     * LLM for a one-line summary. Off unless a project admin turns it
     * on — cast files can contain pasted secrets.
     */
    ai_summaries_enabled: boolean;
}

export interface ProjectMember {
    user_id: string;
    username: string;
    role: "admin" | "operator" | "viewer";
}

export async function listProjects(): Promise<Project[]> {
    const j = await authJSON<{ projects: Project[] }>("/api/v1/projects");
    return j.projects;
}

export async function createProject(name: string, slug: string): Promise<Project> {
    return authJSON<Project>("/api/v1/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, slug }),
    });
}

/**
 * Flip the per-project AI-summary opt-in. Requires project-admin:
 * enabling it sends terminal output to a third party, so it is not a
 * viewer-level decision. Returns the updated project.
 */
export async function setProjectAISummaries(
    pid: string,
    enabled: boolean,
): Promise<Project> {
    return authJSON<Project>(`/api/v1/projects/${pid}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ai_summaries_enabled: enabled }),
    });
}

export async function deleteProject(pid: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}`, { method: "DELETE" });
}

export async function listProjectMembers(pid: string): Promise<ProjectMember[]> {
    const j = await authJSON<{ members: ProjectMember[] }>(`/api/v1/projects/${pid}/members`);
    return j.members;
}

export async function addProjectMember(
    pid: string,
    userID: string,
    role: ProjectMember["role"],
): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/members`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userID, role }),
    });
}

export async function removeProjectMember(pid: string, userID: string): Promise<void> {
    await authFetch(`/api/v1/projects/${pid}/members/${userID}`, { method: "DELETE" });
}
