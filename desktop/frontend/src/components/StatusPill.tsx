import type { ReactNode } from "react";

import { palette, radius } from "../layout/theme";

type Tone = "neutral" | "success" | "warning" | "danger" | "info";

interface Props {
    tone?: Tone;
    children: ReactNode;
}

const toneColor: Record<Tone, string> = {
    neutral: palette.textSecondary,
    success: palette.success,
    warning: palette.warning,
    danger: palette.danger,
    info: palette.info,
};

// StatusPill is a rounded-full status badge: 1px coloured border, same
// coloured text, transparent fill. Replaces ad-hoc <Tag> usage for
// status presentation. Sized to sit comfortably on dense table rows.
export default function StatusPill({ tone = "neutral", children }: Props) {
    const c = toneColor[tone];
    return (
        <span
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                padding: "1px 8px",
                fontSize: 11,
                fontWeight: 500,
                lineHeight: 1.6,
                color: c,
                border: `1px solid ${c}`,
                borderRadius: radius.pill,
                background: "transparent",
                whiteSpace: "nowrap",
            }}
        >
            {children}
        </span>
    );
}
