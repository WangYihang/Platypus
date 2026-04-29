import { authFetch, authJSON } from "../auth";

export interface Project {
    id: string;
    name: string;
    slug: string;
    created_at: string;
    created_by: string;
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
