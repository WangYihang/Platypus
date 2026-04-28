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
// view. Round 1 sized this at 28/600 title + 14 subtitle + 32px
// horizontal padding — "大气", but ate ~150 px of vertical chrome on
// host pages once the tab bar landed. The 2026-04 density pass
// shrinks the title to 18/600, the subtitle to 12, and inlines the
// title row at minHeight 36 so the whole header (incl. tabs) sits
// well under 80 px while still reading as a page identity strip.
export default function PageHeader({ title, subtitle, actions, tabs }: Props) {
    return (
        <header
            style={{
                background: palette.main,
                borderBottom: tabs ? "none" : `1px solid ${palette.border}`,
                padding: `0 ${space[5]}px`,
            }}
        >
            <div
                style={{
                    minHeight: 36,
                    paddingTop: space[2],
                    paddingBottom: tabs ? space[1] : space[2],
                    display: "flex",
                    alignItems: "flex-start",
                    gap: space[3],
                }}
            >
                <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
                    <div
                        style={{
                            color: palette.textPrimary,
                            fontFamily: font.sans,
                            fontWeight: 600,
                            fontSize: 18,
                            lineHeight: 1.2,
                            letterSpacing: -0.1,
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
                                lineHeight: 1.35,
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
