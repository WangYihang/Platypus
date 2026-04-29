import { ReactNode } from "react";

import { palette, space } from "../layout/theme";

// StatusPills renders the inline status row that mockups place beside
// page titles: `Fleet · 10 online · 1 offline · 1 warn`. Each pill is
// a coloured dot + count + label inside a hairline-bordered chip; the
// row is a flex group that sits between the title cluster and the
// PageHeader's tabs / actions zones. Tones map to the same colour
// vocabulary the rest of the UI already uses (success / muted /
// warning / danger / info / brand) so a heterogeneous pill set still
// reads as one cohesive strip.
//
// Empty / zero counts are filtered out by default — a zero pill is
// noise, not signal. Pass `keepZero` if a specific pill should always
// render (e.g. a "0 errors" reassurance pill on a status board).

export type PillTone =
    | "success"
    | "muted"
    | "warning"
    | "danger"
    | "info"
    | "brand";

export interface StatusPill {
    tone: PillTone;
    count: ReactNode;
    label: string;
    keepZero?: boolean;
}

interface Props {
    pills: StatusPill[];
}

const DOT_COLOR: Record<PillTone, string> = {
    success: palette.success,
    muted: palette.textMuted,
    warning: palette.warning,
    danger: palette.danger,
    info: palette.info,
    brand: palette.accent,
};

export default function StatusPills({ pills }: Props) {
    const visible = pills.filter((p) => {
        if (p.keepZero) return true;
        if (typeof p.count === "number") return p.count > 0;
        if (typeof p.count === "string") return p.count.length > 0;
        return p.count != null;
    });
    if (visible.length === 0) return null;
    return (
        <div
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: space[2],
                flexWrap: "nowrap",
            }}
        >
            {visible.map((p, i) => (
                <span
                    key={`${p.label}:${i}`}
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 6,
                        padding: `2px ${space[2]}px`,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 4,
                        color: palette.textSecondary,
                        fontSize: 11,
                        whiteSpace: "nowrap",
                    }}
                >
                    <span
                        aria-hidden
                        style={{
                            width: 6,
                            height: 6,
                            borderRadius: "50%",
                            background: DOT_COLOR[p.tone],
                            flexShrink: 0,
                        }}
                    />
                    <span style={{ color: palette.textPrimary, fontWeight: 600 }}>
                        {p.count}
                    </span>
                    <span>{p.label}</span>
                </span>
            ))}
        </div>
    );
}
