// roles.ts is the canonical vocabulary for user / member roles. Pages
// that render a role label (ProjectMembers, AdminUsers, UserMenu)
// share both the order and the human-readable descriptions from here
// so the tooltips and badges agree.

import type { SessionUser } from "./auth";

export type Role = SessionUser["role"];

// Ordered most → least privileged so a top-down read in tooltips
// or selects communicates "decreasing power". This is the order
// every member-management surface uses.
export const ROLES: readonly Role[] = ["admin", "operator", "viewer"] as const;

// Short, action-oriented descriptions. "operator" is jargon if it
// stands alone; the description spells out what the role can/can't do
// so a new admin doesn't have to read RBAC code to grant the right
// access level.
export const ROLE_DESCRIPTION: Record<Role, string> = {
    admin:
        "Full control: members, project settings, danger-zone actions, and everything Operator and Viewer can do.",
    operator:
        "Day-to-day operation: open shells, run files, manage hosts and sessions. Cannot add/remove members or delete the project.",
    viewer:
        "Read-only: can browse hosts, sessions, and audit history. Cannot run commands, mutate state, or manage access.",
};

// formatRoleSummary returns a single string suitable for a Tooltip
// content prop — newline-joined "Role · description" rows so the
// tooltip lays out as a vertical legend rather than a wall of text.
export function formatRoleSummary(): string {
    return ROLES.map((r) => `${capitalise(r)} · ${ROLE_DESCRIPTION[r]}`).join("\n");
}

function capitalise(s: string): string {
    return s.length === 0 ? s : s[0].toUpperCase() + s.slice(1);
}
