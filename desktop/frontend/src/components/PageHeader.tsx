import { ReactNode } from "react";

import { font, palette, space } from "../layout/theme";

interface Props {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
    // Optional sub-tab bar rendered on the same row as the title (right
    // of the title cluster, left of `actions`). HostView is the only
    // current consumer; the slot is here so future tabbed pages don't
    // need a parallel layout component.
    tabs?: ReactNode;
    // Inline status pills rendered between the title cluster and the
    // tabs / actions zones. Mockups place a row of `· N online · M
    // offline · K warn` after the title — see `<StatusPills>` for the
    // pill row primitive.
    pills?: ReactNode;
}

// PageHeader is the page-identity strip at the top of every main-panel
// view. The single-row layout collapses title + inline subtitle + tabs
// + actions into one ~36 px band — earlier iterations stacked them on
// two or three rows and ate up to 150 px of vertical chrome before any
// content rendered.
//
// Layout (left → right):
//
//   [title  · muted-subtitle]   [tabs (right-aligned, flex-1)]   [actions]
//
// Title and subtitle live on the same line; the subtitle drops off on
// narrow viewports (< 720 px) so the title and actions stay legible.
// The bottom hairline is always drawn; consumers with `tabs` lean on
// the tab underline as a secondary divider but the page-level border
// stays so non-tab pages don't look open at the bottom.
export default function PageHeader({ title, subtitle, actions, tabs, pills }: Props) {
    return (
        <header
            style={{
                background: palette.main,
                borderBottom: `1px solid ${palette.border}`,
                padding: `0 ${space[4]}px`,
                minHeight: 36,
                display: "flex",
                alignItems: "center",
                gap: space[3],
            }}
        >
            <div
                style={{
                    minWidth: 0,
                    display: "flex",
                    alignItems: "baseline",
                    gap: space[2],
                    flexShrink: 1,
                }}
            >
                <div
                    style={{
                        color: palette.textPrimary,
                        fontFamily: font.mono,
                        fontWeight: 600,
                        fontSize: 14,
                        lineHeight: 1.3,
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
                        className="hidden md:block"
                        style={{
                            color: palette.textSecondary,
                            fontSize: 12,
                            lineHeight: 1.35,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                        }}
                    >
                        · {subtitle}
                    </div>
                )}
            </div>
            {pills && (
                <div
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: space[2],
                        flexShrink: 0,
                    }}
                >
                    {pills}
                </div>
            )}
            {tabs && (
                <div
                    style={{
                        flex: 1,
                        minWidth: 0,
                        display: "flex",
                        justifyContent: "flex-end",
                        overflow: "hidden",
                    }}
                >
                    {tabs}
                </div>
            )}
            {actions && (
                <div
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: space[2],
                        flexShrink: 0,
                        marginLeft: tabs ? 0 : "auto",
                    }}
                >
                    {actions}
                </div>
            )}
        </header>
    );
}
