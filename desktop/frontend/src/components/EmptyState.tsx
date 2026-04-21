import type { ReactNode } from "react";

import { palette, space } from "../layout/theme";

interface Props {
    icon?: ReactNode;
    title?: ReactNode;
    description?: ReactNode;
    action?: ReactNode;
    fill?: boolean;
}

// EmptyState is the centred placeholder used when a list/table has no
// rows or a panel has no selection. Keep copy short — title is one
// line, description is one sentence. Replaces ad-hoc Alert info blocks.
export default function EmptyState({
    icon,
    title,
    description,
    action,
    fill = false,
}: Props) {
    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                gap: space[3],
                textAlign: "center",
                maxWidth: 360,
                margin: "0 auto",
                padding: `${space[8]}px ${space[5]}px`,
                minHeight: fill ? "100%" : undefined,
            }}
        >
            {icon && (
                <div style={{ color: palette.textMuted, fontSize: 28 }}>{icon}</div>
            )}
            {title && (
                <div
                    style={{
                        color: palette.textPrimary,
                        fontWeight: 600,
                        fontSize: 14,
                    }}
                >
                    {title}
                </div>
            )}
            {description && (
                <div style={{ color: palette.textSecondary, fontSize: 13, lineHeight: 1.5 }}>
                    {description}
                </div>
            )}
            {action && <div style={{ marginTop: space[2] }}>{action}</div>}
        </div>
    );
}
