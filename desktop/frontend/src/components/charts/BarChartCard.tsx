import { ReactNode } from "react";
import {
    Bar,
    BarChart,
    CartesianGrid,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";

import Card from "../Card";
import EmptyState from "../EmptyState";
import ChartContainer from "./ChartContainer";
import { font, palette, space } from "../../layout/theme";

export interface BarPoint {
    label: string;
    value: number;
}

interface Props {
    title: ReactNode;
    seriesLabel?: ReactNode;
    data: BarPoint[];
    color?: string;
    height?: number;
}

// BarChartCard renders a horizontal bar chart inside our Card primitive.
// Used on ProjectOverview for "top hosts by session count" and similar
// rank views. Pass `seriesLabel` to render an inline coloured-dot
// legend above the bars — mirrors LineChartCard's contract.
export default function BarChartCard({
    title,
    seriesLabel,
    data,
    color = palette.textPrimary,
    height = 220,
}: Props) {
    return (
        <Card header={title}>
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
                    <EmptyState title="No data" description="Nothing to rank yet." />
                </div>
            ) : (
                <div style={{ width: "100%", height }}>
                    <ChartContainer height={height}>
                        <BarChart
                            data={data}
                            layout="vertical"
                            margin={{ top: 4, right: 8, bottom: 0, left: 0 }}
                        >
                            <CartesianGrid
                                stroke={palette.border}
                                strokeDasharray="3 3"
                                horizontal={false}
                            />
                            <XAxis
                                type="number"
                                stroke={palette.textMuted}
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.mono }}
                                tickLine={false}
                                axisLine={{ stroke: palette.border }}
                                allowDecimals={false}
                            />
                            <YAxis
                                type="category"
                                dataKey="label"
                                stroke={palette.textMuted}
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.mono }}
                                tickLine={false}
                                axisLine={false}
                                width={120}
                            />
                            <Tooltip
                                contentStyle={{
                                    background: palette.surface,
                                    border: `1px solid ${palette.borderStrong}`,
                                    borderRadius: 6,
                                    color: palette.textPrimary,
                                    fontSize: 12,
                                }}
                                cursor={{ fill: palette.surfaceHover }}
                            />
                            <Bar dataKey="value" fill={color} radius={[0, 3, 3, 0]} />
                        </BarChart>
                    </ChartContainer>
                </div>
            )}
        </Card>
    );
}
