import { ReactNode } from "react";

import { palette, space } from "../layout/theme";

interface Props {
    left?: ReactNode;
    right?: ReactNode;
}

// Toolbar is the row that sits between PageHeader and the page body on
// list pages. Search input + filter chips on the left, refresh / view
// toggles on the right. Hairline under it gives a clean break before
// the table card.
export default function Toolbar({ left, right }: Props) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[3],
                padding: `${space[3]}px ${space[8]}px`,
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <div style={{ flex: 1, display: "flex", alignItems: "center", gap: space[2], minWidth: 0 }}>
                {left}
            </div>
            {right && (
                <div style={{ display: "flex", alignItems: "center", gap: space[2], flexShrink: 0 }}>
                    {right}
                </div>
            )}
        </div>
    );
}
