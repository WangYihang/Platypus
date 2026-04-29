import { CSSProperties, ReactNode } from "react";

import { space } from "../layout/theme";
import PageHeader from "./PageHeader";

type SpaceKey = keyof typeof space;

interface Props {
    title: ReactNode;
    subtitle?: ReactNode;
    actions?: ReactNode;
    tabs?: ReactNode;
    pills?: ReactNode;
    // Body padding token. Pages used to spell `padding: space[8]`
    // inline (32 px) or `space[6]` / `space[4]`; centralising here so
    // a future density tweak edits one component, not seventeen.
    // `0` opts out of padding entirely (FleetPage / AuditPage manage
    // their own panel layout inside the body).
    bodyPadding?: SpaceKey | 0;
    // Pass-through for the body wrapper className. Used by pages that
    // need a different overflow/scroll regime than the default
    // (`overflow: auto`).
    bodyClassName?: string;
    bodyStyle?: CSSProperties;
    children: ReactNode;
}

// PageShell is the page-identity + scrollable-body wrapper that 17
// pages used to spell out by hand:
//
//   <div style={{display:"flex", flexDirection:"column", height:"100%"}}>
//     <PageHeader title=... subtitle=... actions=... tabs=... />
//     <div style={{flex:1, overflow:"auto", padding:space[N]}}>
//       {body}
//     </div>
//   </div>
//
// One primitive removes ~200 LOC of duplication and means the next
// chrome tweak (e.g. dropping the body-padding when a page introduces
// its own toolbar) lands in a single component.
export default function PageShell({
    title,
    subtitle,
    actions,
    tabs,
    pills,
    bodyPadding = 4,
    bodyClassName,
    bodyStyle,
    children,
}: Props) {
    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader title={title} subtitle={subtitle} actions={actions} tabs={tabs} pills={pills} />
            <div
                className={bodyClassName}
                style={{
                    flex: 1,
                    overflow: "auto",
                    padding: bodyPadding === 0 ? 0 : space[bodyPadding],
                    ...bodyStyle,
                }}
            >
                {children}
            </div>
        </div>
    );
}
