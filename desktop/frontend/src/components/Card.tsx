import type { CSSProperties, ReactNode } from "react";

import { palette, radius, space } from "../layout/theme";

type SpaceKey = keyof typeof space;

interface Props {
    children?: ReactNode;
    header?: ReactNode;
    footer?: ReactNode;
    padding?: SpaceKey | 0;
    interactive?: boolean;
    className?: string;
    style?: CSSProperties;
    onClick?: () => void;
}

// Card is the Vercel-style surface primitive: 1px hairline border on
// the surface tier (one shade lighter than the page background), 6px
// rounded corners, no shadow. Header/footer are optional and sit on
// their own padded rows separated by hairlines from the body.
export default function Card({
    children,
    header,
    footer,
    padding = 5,
    interactive = false,
    className,
    style,
    onClick,
}: Props) {
    const pad = padding === 0 ? 0 : space[padding];
    const cls = ["pl-card", interactive ? "pl-card--interactive" : "", className]
        .filter(Boolean)
        .join(" ");

    return (
        <div
            className={cls}
            onClick={onClick}
            role={interactive && onClick ? "button" : undefined}
            tabIndex={interactive && onClick ? 0 : undefined}
            onKeyDown={
                interactive && onClick
                    ? (e) => {
                          if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              onClick();
                          }
                      }
                    : undefined
            }
            style={{
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                overflow: "hidden",
                ...style,
            }}
        >
            {header && (
                <div
                    style={{
                        padding: `${space[3]}px ${space[5]}px`,
                        borderBottom: `1px solid ${palette.border}`,
                        color: palette.textPrimary,
                        fontWeight: 600,
                        fontSize: 13,
                    }}
                >
                    {header}
                </div>
            )}
            {children !== undefined && children !== null && (
                <div style={{ padding: pad }}>{children}</div>
            )}
            {footer && (
                <div
                    style={{
                        padding: `${space[3]}px ${space[5]}px`,
                        borderTop: `1px solid ${palette.border}`,
                        color: palette.textSecondary,
                        fontSize: 12,
                    }}
                >
                    {footer}
                </div>
            )}
        </div>
    );
}
