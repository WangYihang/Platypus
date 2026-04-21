import { ReactNode } from "react";
import {
    CartesianGrid,
    Line,
    LineChart,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";

import Card from "../Card";
import EmptyState from "../EmptyState";
import { font, palette, space } from "../../layout/theme";

export interface LinePoint {
    label: string;
    value: number;
    ts?: number;
}

interface Props {
    title: ReactNode;
    hint?: ReactNode;
    data: LinePoint[];
    color?: string;
    height?: number;
    formatY?: (v: number) => string;
}

// LineChartCard wraps Recharts <LineChart> in our Card primitive with
// dashboard-friendly axis styling. Used for the "sessions over 24h"
// time series on ProjectOverview.
export default function LineChartCard({
    title,
    hint,
    data,
    color = palette.successDot,
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
            {data.length === 0 ? (
                <div style={{ height }}>
                    <EmptyState title="No data" description="Nothing to plot for this window." />
                </div>
            ) : (
                <div style={{ width: "100%", height }}>
                    <ResponsiveContainer>
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
                    </ResponsiveContainer>
                </div>
            )}
        </Card>
    );
}
