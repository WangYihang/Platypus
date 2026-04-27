// Enrollment status vocabulary, shared across the install-command and
// PAT lifecycles. Two parallel maps so the back-end's lifecycle words
// ("pending" / "consumed") never reach the screen as-is — users read
// "pending" + green and conclude the action has succeeded; the actual
// success state is "consumed", which we surface as "Used" in green.
//
// Tones map to StatusPill's existing tone vocabulary
// (desktop/frontend/src/components/StatusPill.tsx). "pending" used to
// be "neutral" (gray), which read as inert/disabled — operators saw
// "Unused" + gray and assumed the install command had gone stale.
// Switching to "info" (blue) signals "ready, waiting for the agent",
// keeping "consumed" / "expired" / "revoked" for terminal states.

import type { ReactNode } from "react";

export type EnrollmentStatus = "pending" | "consumed" | "expired" | "revoked";

export type StatusTone = "neutral" | "success" | "warning" | "danger" | "info";

export const STATUS_LABEL: Record<EnrollmentStatus, string> = {
    pending: "Unused",
    consumed: "Used",
    expired: "Expired",
    revoked: "Revoked",
};

export const STATUS_TONE: Record<EnrollmentStatus, StatusTone> = {
    pending: "info",
    consumed: "success",
    expired: "warning",
    revoked: "danger",
};

// Re-exported as a function so call sites stay short. The component
// import lives in the consuming pages (EnrollmentPage / PATs list)
// so this module stays a pure-data leaf — easier to unit-test.
export type StatusBadgeRenderer = (status: EnrollmentStatus) => ReactNode;
