import { ReactNode } from "react";
import {
    CartesianGrid,
    Line,
    LineChart,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";

import Card from "../Card";
import EmptyState from "../EmptyState";
import ChartContainer from "./ChartContainer";
import { font, palette, space } from "../../layout/theme";

export interface LinePoint {
    label: string;
    value: number;
    ts?: number;
}

interface Props {
    title: ReactNode;
    hint?: ReactNode;
    seriesLabel?: ReactNode;
    data: LinePoint[];
    color?: string;
    height?: number;
    formatY?: (v: number) => string;
}

// LineChartCard wraps Recharts <LineChart> in our Card primitive with
// dashboard-friendly axis styling. Used for the "sessions over 24h"
// time series on ProjectOverview.
//
// `seriesLabel` (optional) renders an inline coloured-dot legend
// directly above the chart area. Recharts' built-in <Legend> looks
// heavy on a single-series chart, but readers still benefit from a
// tiny "Sessions per hour" label that explains the dimension. Pass
// the same dimension noun the consumer wants on the y axis.
export default function LineChartCard({
    title,
    hint,
    seriesLabel,
    data,
    color = palette.info,
    height = 220,
    formatY,
}: Props) {
    return (
        <Card header={title}>
            {hint && (
                <div
                    style={{
                        marginTop: -space[2],
                        marginBottom: space[3],
                        color: palette.textSecondary,
                        fontSize: 12,
                    }}
                >
                    {hint}
                </div>
            )}
            {seriesLabel && (
                <div
                    data-testid="chart-legend"
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 6,
                        marginBottom: space[2],
                        color: palette.textSecondary,
                        fontSize: 12,
                    }}
                >
                    <span
                        aria-hidden
                        style={{
                            display: "inline-block",
                            width: 8,
                            height: 8,
                            borderRadius: 999,
                            background: color,
                        }}
                    />
                    <span>{seriesLabel}</span>
                </div>
            )}
            {data.length === 0 ? (
                <div style={{ height }}>
                    <EmptyState title="No data" description="Nothing to plot for this window." />
                </div>
            ) : (
                <div style={{ width: "100%", height }}>
                    <ChartContainer height={height}>
                        <LineChart
                            data={data}
                            margin={{ top: 4, right: 8, bottom: 0, left: -16 }}
                        >
                            <CartesianGrid
                                stroke={palette.border}
                                strokeDasharray="3 3"
                                vertical={false}
                            />
                            <XAxis
                                dataKey="label"
                                stroke={palette.textMuted}
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.sans }}
                                tickLine={false}
                                axisLine={{ stroke: palette.border }}
                            />
                            <YAxis
                                stroke={palette.textMuted}
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.sans }}
                                tickLine={false}
                                axisLine={false}
                                allowDecimals={false}
                                tickFormatter={formatY}
                                width={40}
                            />
                            <Tooltip
                                contentStyle={{
                                    background: palette.surface,
                                    border: `1px solid ${palette.borderStrong}`,
                                    borderRadius: 6,
                                    color: palette.textPrimary,
                                    fontSize: 12,
                                }}
                                cursor={{ stroke: palette.borderStrong, strokeDasharray: "3 3" }}
                                labelStyle={{ color: palette.textMuted, marginBottom: 4 }}
                                formatter={(v) => [
                                    formatY && typeof v === "number" ? formatY(v) : String(v),
                                    "value",
                                ]}
                            />
                            <Line
                                type="monotone"
                                dataKey="value"
                                stroke={color}
                                strokeWidth={2}
                                dot={false}
                                activeDot={{ r: 4, fill: color }}
                            />
                        </LineChart>
                    </ChartContainer>
                </div>
            )}
        </Card>
    );
}
