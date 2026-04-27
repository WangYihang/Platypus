import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import BarChartCard from "./BarChartCard";

// Same contract as LineChartCard's seriesLabel — a tiny inline
// legend names the dimension when the title alone is ambiguous.

describe("<BarChartCard>", () => {
    it("renders the seriesLabel as an inline legend when provided", () => {
        render(
            <BarChartCard
                title="Top hosts"
                seriesLabel="Hosts by session count"
                data={[{ label: "host-1", value: 5 }]}
            />,
        );
        expect(screen.getByText("Hosts by session count")).toBeInTheDocument();
        expect(screen.getByTestId("chart-legend")).toBeInTheDocument();
    });

    it("does NOT render a legend when seriesLabel is omitted", () => {
        render(
            <BarChartCard
                title="Top hosts"
                data={[{ label: "host-1", value: 5 }]}
            />,
        );
        expect(screen.queryByTestId("chart-legend")).toBeNull();
    });
});
