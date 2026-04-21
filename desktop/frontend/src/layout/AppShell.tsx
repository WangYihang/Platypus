import { ReactNode } from "react";

import { layout, palette } from "./theme";

// AppShell is the 4-region grid wrapping the whole post-login UI. The
// regions (profile rail, sidebar, main panel, detail rail) take props
// so each grows independently — this component is purely structural,
// it doesn't know about Projects / Hosts / Sessions.
//
// Detail rail is collapsible: pass `detail={undefined}` (or nothing) and
// the grid column disappears, giving the main area the full remaining
// width.
interface Props {
    profileRail: ReactNode;
    sidebar: ReactNode;
    main: ReactNode;
    detail?: ReactNode;
}

export default function AppShell({ profileRail, sidebar, main, detail }: Props) {
    const gridTemplateColumns = detail
        ? `${layout.profileRailWidth}px ${layout.sidebarWidth}px 1fr ${layout.detailRailWidth}px`
        : `${layout.profileRailWidth}px ${layout.sidebarWidth}px 1fr`;

    return (
        <div
            style={{
                display: "grid",
                gridTemplateColumns,
                height: "100vh",
                background: palette.main,
                color: palette.textPrimary,
                overflow: "hidden",
            }}
        >
            <section style={regionStyle(palette.rail)}>{profileRail}</section>
            <section style={regionStyle(palette.sidebar, { borderRight: true })}>
                {sidebar}
            </section>
            <section style={{ ...regionStyle(palette.main), overflow: "hidden" }}>
                {main}
            </section>
            {detail && (
                <section style={regionStyle(palette.detailRail, { borderLeft: true })}>
                    {detail}
                </section>
            )}
        </div>
    );
}

function regionStyle(
    bg: string,
    opts?: { borderLeft?: boolean; borderRight?: boolean }
): React.CSSProperties {
    return {
        background: bg,
        height: "100vh",
        overflow: "auto",
        borderLeft: opts?.borderLeft ? `1px solid ${palette.border}` : undefined,
        borderRight: opts?.borderRight ? `1px solid ${palette.border}` : undefined,
    };
}
