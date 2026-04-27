// Member-removal warnings for ProjectMembers. Pure data so it can be
// unit-tested without touching the React tree. The dialog renders the
// returned string (when non-null) above the standard description.

interface RemovalContext {
    memberCount: number;
    isProjectAdmin: boolean;
}

// memberRemovalWarning surfaces extra context for the more jarring
// removal cases: zero-member projects, and losing the last project
// admin. Ordinary removals (operator/viewer with siblings) need no
// extra copy — the existing description already explains the impact.
export function memberRemovalWarning({
    memberCount,
    isProjectAdmin,
}: RemovalContext): string | null {
    if (memberCount <= 1) {
        return "This is the last member of the project — the project will have no explicit members after removal. Global admins can still access it.";
    }
    if (isProjectAdmin) {
        return "This member is a project admin. Removing them may leave the project without a project-level admin.";
    }
    return null;
}
