import { ReactNode } from "react";
import {
    Bar,
    BarChart,
    CartesianGrid,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";

import Card from "../Card";
import EmptyState from "../EmptyState";
import { font, palette } from "../../layout/theme";

export interface BarPoint {
    label: string;
    value: number;
}

interface Props {
    title: ReactNode;
    data: BarPoint[];
    color?: string;
    height?: number;
}

// BarChartCard renders a horizontal bar chart inside our Card primitive.
// Used on ProjectOverview for "top hosts by session count" and similar
// rank views.
export default function BarChartCard({
    title,
    data,
    color = palette.textPrimary,
    height = 220,
}: Props) {
    return (
        <Card header={title}>
            {data.length === 0 ? (
                <div style={{ height }}>
                    <EmptyState title="No data" description="Nothing to rank yet." />
                </div>
            ) : (
                <div style={{ width: "100%", height }}>
                    <ResponsiveContainer>
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
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.sans }}
                                tickLine={false}
                                axisLine={{ stroke: palette.border }}
                                allowDecimals={false}
                            />
                            <YAxis
                                type="category"
                                dataKey="label"
                                stroke={palette.textMuted}
                                tick={{ fill: palette.textMuted, fontSize: 11, fontFamily: font.sans }}
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
                    </ResponsiveContainer>
                </div>
            )}
        </Card>
    );
}
