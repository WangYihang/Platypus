import { palette } from "../layout/theme";

interface Props {
    values: number[];
    width?: number;
    height?: number;
    color?: string;
    title?: string;
}

// Sparkline renders a tiny inline polyline of a numeric series.
// It's deliberately self-contained (no recharts) so the status bar
// can show many of them at 1 Hz without dragging in a heavyweight
// chart engine. Empty / single-sample series render as a flat line
// at mid-height so the chip space doesn't collapse during the first
// second after mount.
export default function Sparkline({
    values,
    width = 48,
    height = 14,
    color = palette.info,
    title,
}: Props) {
    const n = values.length;
    if (n === 0) {
        return (
            <svg
                data-testid="sparkline"
                width={width}
                height={height}
                role="img"
                aria-label={title}
                style={{ display: "inline-block", verticalAlign: "middle" }}
            >
                <title>{title}</title>
                <line
                    x1={0}
                    y1={height / 2}
                    x2={width}
                    y2={height / 2}
                    stroke={palette.border}
                    strokeWidth={1}
                />
            </svg>
        );
    }

    let min = Infinity;
    let max = -Infinity;
    for (const v of values) {
        if (v < min) min = v;
        if (v > max) max = v;
    }
    const span = max - min || 1;

    const stepX = n > 1 ? width / (n - 1) : 0;
    const points = values
        .map((v, i) => {
            const x = i * stepX;
            // Pad the y axis by 1px so the line never sits flush
            // against the chip's top/bottom edge.
            const y = height - 1 - ((v - min) / span) * (height - 2);
            return `${x.toFixed(1)},${y.toFixed(1)}`;
        })
        .join(" ");

    return (
        <svg
            data-testid="sparkline"
            width={width}
            height={height}
            role="img"
            aria-label={title}
            style={{ display: "inline-block", verticalAlign: "middle" }}
        >
            <title>{title}</title>
            <polyline
                points={points}
                fill="none"
                stroke={color}
                strokeWidth={1}
                strokeLinejoin="round"
                strokeLinecap="round"
            />
        </svg>
    );
}
