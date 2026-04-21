import { ReactNode } from "react";

import Mono from "./Mono";
import StatusDot from "./StatusDot";
import { palette, space } from "../layout/theme";

export interface ActivityItem {
    id: string;
    when: string;
    status: "online" | "offline" | "warning" | "error";
    actor: ReactNode;
    verb: ReactNode;
    target?: ReactNode;
    onClick?: () => void;
}

interface Props {
    items: ActivityItem[];
    emptyHint?: string;
}

// ActivityFeed renders a vertical list of recent events: dot + when +
// "actor verb target" line, with optional click-through to the
// referenced entity. Used on ProjectOverview "Recent activity".
export default function ActivityFeed({ items, emptyHint }: Props) {
    if (items.length === 0) {
        return (
            <div style={{ color: palette.textMuted, fontSize: 13, textAlign: "center", padding: space[4] }}>
                {emptyHint ?? "No activity yet."}
            </div>
        );
    }
    return (
        <div style={{ display: "flex", flexDirection: "column" }}>
            {items.map((it, i) => (
                <div
                    key={it.id}
                    role={it.onClick ? "button" : undefined}
                    onClick={it.onClick}
                    style={{
                        display: "grid",
                        gridTemplateColumns: "16px auto 1fr",
                        alignItems: "center",
                        gap: space[3],
                        padding: `${space[2]}px 0`,
                        borderBottom:
                            i < items.length - 1 ? `1px solid ${palette.border}` : "none",
                        cursor: it.onClick ? "pointer" : "default",
                    }}
                >
                    <span style={{ display: "inline-flex", justifyContent: "center" }}>
                        <StatusDot status={it.status} size={6} />
                    </span>
                    <Mono size={11} color={palette.textMuted}>
                        {it.when}
                    </Mono>
                    <span style={{ fontSize: 13, color: palette.textPrimary, minWidth: 0 }}>
                        {it.actor} <span style={{ color: palette.textMuted }}>{it.verb}</span>{" "}
                        {it.target}
                    </span>
                </div>
            ))}
        </div>
    );
}
