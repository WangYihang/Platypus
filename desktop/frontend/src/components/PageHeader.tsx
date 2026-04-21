import { ReactNode } from "react";

import { font, palette, space } from "../layout/theme";

interface Props {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
    // Optional sub-tab bar rendered directly under the title row, sharing
    // the same header surface. When set, the bottom hairline is suppressed
    // so the Antd underline tab ink-bar serves as the divider.
    tabs?: ReactNode;
}

// PageHeader is the page-identity strip at the top of every main-panel
// view. Bigger than Round 1's MainHeader (28/600 title, 14 subtitle,
// 32px padding) — this is the "大气" promise: every page reads as a
// proper destination, not a panel inside a chrome box.
export default function PageHeader({ title, subtitle, actions, tabs }: Props) {
    return (
        <header
            style={{
                background: palette.main,
                borderBottom: tabs ? "none" : `1px solid ${palette.border}`,
                padding: `0 ${space[8]}px`,
            }}
        >
            <div
                style={{
                    minHeight: 80,
                    paddingTop: space[5],
                    paddingBottom: tabs ? space[3] : space[5],
                    display: "flex",
                    alignItems: "flex-start",
                    gap: space[4],
                }}
            >
                <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
                    <div
                        style={{
                            color: palette.textPrimary,
                            fontFamily: font.sans,
                            fontWeight: 600,
                            fontSize: 28,
                            lineHeight: 1.15,
                            letterSpacing: -0.3,
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
                                fontSize: 14,
                                lineHeight: 1.4,
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
                    <div
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: space[2],
                            flexShrink: 0,
                        }}
                    >
                        {actions}
                    </div>
                )}
            </div>
            {tabs}
        </header>
    );
}
