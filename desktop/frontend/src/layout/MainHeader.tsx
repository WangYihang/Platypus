import { ReactNode } from "react";

import { layout, palette } from "./theme";

interface Props {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
}

// MainHeader is the 48px strip that sits at the top of every main-panel
// view. Title + subtitle on the left, actions (refresh, create, etc.)
// on the right. Kept header-only — sub-tabs / filters should live in
// the view below this bar so the identity of "what am I looking at"
// is fixed.
export default function MainHeader({ title, subtitle, actions }: Props) {
    return (
        <header
            style={{
                height: layout.mainHeaderHeight,
                minHeight: layout.mainHeaderHeight,
                padding: "0 20px",
                display: "flex",
                alignItems: "center",
                gap: 16,
                borderBottom: `1px solid ${palette.border}`,
                background: palette.main,
            }}
        >
            <div
                style={{
                    display: "flex",
                    flexDirection: "column",
                    flex: 1,
                    minWidth: 0,
                }}
            >
                <div
                    style={{
                        color: palette.textPrimary,
                        fontWeight: 600,
                        fontSize: 14,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                    }}
                >
                    {title}
                </div>
                {subtitle && (
                    <div
                        style={{
                            color: palette.textSecondary,
                            fontSize: 12,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                        }}
                    >
                        {subtitle}
                    </div>
                )}
            </div>
            {actions && (
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>{actions}</div>
            )}
        </header>
    );
}
