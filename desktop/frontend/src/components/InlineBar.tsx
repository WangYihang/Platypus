import { CSSProperties } from "react";

import { palette } from "../layout/theme";
import Mono from "./Mono";

// InlineBar renders the thin "[▰▰▰░░] 42%" style inline progress bar
// the redesign uses for CPU / MEM / DISK on every host listing. The
// bar fill colour is threshold-driven so a glance at the row is
// enough to tell green / busy / hot / saturated without reading the
// number:
//
//   value <  50 → success (green)
//   value <  70 → warning (amber)   — borrowed accent
//   value <  80 → text-primary (white) — keeping the row monochrome
//   value >= 80 → danger (the saturated pink the mockup uses)
//
// Callers can override `color` for non-percentage uses (e.g. a
// disk-usage bar that should always read green until 95%).
//
// The component renders `<div role="progressbar" aria-valuenow={…}>`
// + an `<Mono>` count label so screenreaders + the e2e specs both
// pick up the value without ambiguity.

interface Props {
    value: number;       // 0–100
    width?: number;      // default 64 px
    color?: string;      // override threshold-driven hue
    showText?: boolean;  // default true; renders `<Mono>{N}%</Mono>` to the right
    label?: string;      // optional aria-label override (default "Usage")
    style?: CSSProperties;
    "data-testid"?: string;
}

function thresholdColor(value: number): string {
    if (value < 50) return palette.success;
    if (value < 70) return palette.warning;
    if (value < 80) return palette.textPrimary;
    return palette.danger;
}

export default function InlineBar({
    value,
    width = 64,
    color,
    showText = true,
    label = "Usage",
    style,
    "data-testid": dataTestId,
}: Props) {
    const clamped = Math.max(0, Math.min(100, value));
    const fill = color ?? thresholdColor(clamped);
    return (
        <span
            data-testid={dataTestId}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                ...style,
            }}
        >
            <span
                role="progressbar"
                aria-valuenow={clamped}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={label}
                style={{
                    display: "inline-block",
                    width,
                    height: 4,
                    background: palette.border,
                    borderRadius: 2,
                    overflow: "hidden",
                    flexShrink: 0,
                }}
            >
                <span
                    aria-hidden
                    style={{
                        display: "block",
                        width: `${clamped}%`,
                        height: "100%",
                        background: fill,
                        transition: "width 200ms ease-out, background 200ms ease-out",
                    }}
                />
            </span>
            {showText && (
                <Mono size={11} color={palette.textPrimary}>
                    {`${Math.round(clamped)}%`}
                </Mono>
            )}
        </span>
    );
}
