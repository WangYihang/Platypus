import type { ReactNode } from "react";

import { palette, space } from "../layout/theme";

export interface DataListItem {
    label: ReactNode;
    value: ReactNode;
}

interface Props {
    items: DataListItem[];
    columnLabelWidth?: number;
}

// DataList renders a definition-list pattern: muted label on the left,
// primary value on the right, hairline separator between rows. Replaces
// the Antd <Descriptions bordered /> blocks which look chunky next to
// Vercel hairline cards.
export default function DataList({ items, columnLabelWidth = 140 }: Props) {
    return (
        <div style={{ display: "flex", flexDirection: "column" }}>
            {items.map((item, i) => (
                <div
                    key={i}
                    style={{
                        display: "grid",
                        gridTemplateColumns: `${columnLabelWidth}px 1fr`,
                        gap: space[4],
                        padding: `${space[3]}px 0`,
                        borderBottom:
                            i < items.length - 1
                                ? `1px solid ${palette.border}`
                                : "none",
                        fontSize: 13,
                        alignItems: "baseline",
                    }}
                >
                    <div style={{ color: palette.textMuted }}>{item.label}</div>
                    <div style={{ color: palette.textPrimary, minWidth: 0, wordBreak: "break-word" }}>
                        {item.value}
                    </div>
                </div>
            ))}
        </div>
    );
}
