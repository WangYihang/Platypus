// Shared <Badge> primitive — collapses six near-identical *Badge
// components (StateBadge in two Services tabs, StateBadge in Tasks,
// PriorityBadge in Logs, RiskBadge in FileCaps, ActionBadge in
// Firewall) into a single visual atom.
//
// Per-tab business logic stays where it belongs (each plugin keeps
// its own Record<string, BadgeTone> mapping field-value → semantic
// tone); only the rendering is centralised.

import type { ReactNode } from "react";

import { palette } from "../../../../layout/theme";

export type BadgeTone =
    | "success"
    | "info"
    | "warning"
    | "danger"
    | "muted";

export interface BadgeProps {
    tone: BadgeTone;
    /**
     * Outline shape:
     *   - "pill" (radius=999) — the Services-style state pill.
     *   - "tag"  (radius=4)   — the Firewall/Tasks/Priority style.
     * Defaults to "pill" since five of the six existing call sites
     * use it.
     */
    shape?: "pill" | "tag";
    children: ReactNode;
}

const TONE_BG: Record<BadgeTone, string> = {
    success: palette.success,
    info: palette.info,
    warning: palette.warning,
    danger: palette.danger,
    muted: palette.textMuted,
};

export function Badge({ tone, shape = "pill", children }: BadgeProps) {
    return (
        <span
            data-tone={tone}
            data-shape={shape}
            style={{
                display: "inline-block",
                padding: "2px 8px",
                borderRadius: shape === "pill" ? 999 : 4,
                fontSize: 11,
                fontWeight: 600,
                color: "#fff",
                background: TONE_BG[tone],
            }}
        >
            {children}
        </span>
    );
}
