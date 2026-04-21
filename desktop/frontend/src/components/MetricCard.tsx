import type { ReactNode } from "react";

import { font, palette, space } from "../layout/theme";
import Card from "./Card";

interface Props {
    label: ReactNode;
    value: ReactNode;
    hint?: ReactNode;
    accent?: "default" | "success" | "warning" | "danger";
}

// MetricCard is the Vercel-style stat tile: tracked uppercase label,
// large display value, optional muted hint underneath. Built on Card so
// it inherits the surface treatment.
export default function MetricCard({ label, value, hint, accent = "default" }: Props) {
    const valueColor =
        accent === "success"
            ? palette.successDot
            : accent === "warning"
            ? palette.warning
            : accent === "danger"
            ? palette.danger
            : palette.textPrimary;

    return (
        <Card padding={5}>
            <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
                <div
                    style={{
                        color: palette.textMuted,
                        fontSize: 11,
                        fontWeight: 500,
                        letterSpacing: 0.5,
                        textTransform: "uppercase",
                    }}
                >
                    {label}
                </div>
                <div
                    style={{
                        color: valueColor,
                        fontFamily: font.sans,
                        fontWeight: 600,
                        fontSize: 32,
                        lineHeight: 1.1,
                    }}
                >
                    {value}
                </div>
                {hint && (
                    <div style={{ color: palette.textSecondary, fontSize: 12 }}>{hint}</div>
                )}
            </div>
        </Card>
    );
}
