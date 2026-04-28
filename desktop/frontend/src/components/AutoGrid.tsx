import { CSSProperties, ReactNode } from "react";

import { space } from "../layout/theme";

type SpaceKey = keyof typeof space;

interface Props {
    // Minimum column width in pixels. The grid lays out as
    // `repeat(auto-fit, minmax(<minSize>px, 1fr))` — columns claim at
    // least `minSize` px and grow to fill the container.
    minSize: number;
    gap?: SpaceKey;
    className?: string;
    style?: CSSProperties;
    "data-testid"?: string;
    children: ReactNode;
}

// AutoGrid wraps the `repeat(auto-fit, minmax(N, 1fr))` pattern that
// five call sites used to spell out inline (HostView KPI strip + info
// detail grid; ProjectOverview KPI cards / quick-actions / activity
// meta). One primitive means a future tweak to the breakpoint logic
// or gap rhythm lands in one component.
export default function AutoGrid({
    minSize,
    gap = 3,
    className,
    style,
    "data-testid": dataTestid,
    children,
}: Props) {
    return (
        <div
            className={className}
            data-testid={dataTestid}
            style={{
                display: "grid",
                gridTemplateColumns: `repeat(auto-fit, minmax(${minSize}px, 1fr))`,
                gap: space[gap],
                ...style,
            }}
        >
            {children}
        </div>
    );
}
