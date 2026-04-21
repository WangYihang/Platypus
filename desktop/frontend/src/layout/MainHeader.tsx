import { ReactNode } from "react";

import { font, layout, palette, space } from "./theme";

interface Props {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
    // Optional sub-tab bar rendered directly under the title row, sharing
    // the same header surface. The hairline below the row only renders
    // when no tabs slot is provided — when tabs ARE provided, the Antd
    // Tabs ink bar provides the visual separator.
    tabs?: ReactNode;
}

// MainHeader is the page-identity strip at the top of every main-panel
// view. Title + subtitle on the left, actions on the right. Optional
// tabs slot lets pages (HostView) stack a Vercel-style underline tab
// row directly under the title without breaking the surface.
export default function MainHeader({ title, subtitle, actions, tabs }: Props) {
    return (
        <header
            style={{
                background: palette.main,
                borderBottom: tabs ? "none" : `1px solid ${palette.border}`,
            }}
        >
            <div
                style={{
                    height: layout.mainHeaderHeight,
                    minHeight: layout.mainHeaderHeight,
                    padding: `0 ${space[6]}px`,
                    display: "flex",
                    alignItems: "center",
                    gap: space[4],
                }}
            >
                <div
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        flex: 1,
                        minWidth: 0,
                        gap: 2,
                    }}
                >
                    <div
                        style={{
                            color: palette.textPrimary,
                            fontFamily: font.sans,
                            fontWeight: 600,
                            fontSize: 18,
                            lineHeight: 1.25,
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
                                fontSize: 13,
                                lineHeight: 1.3,
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
                        }}
                    >
                        {actions}
                    </div>
                )}
            </div>
            {tabs && (
                <div style={{ padding: `0 ${space[6]}px` }}>{tabs}</div>
            )}
        </header>
    );
}
