import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import LineChartCard from "./LineChartCard";

// LineChartCard's title prop names the card; the chart inside used
// to have no visible legend or series label, so a reader had to
// infer "what dimension is this?" from the title alone. The
// seriesLabel prop drops a tiny inline legend (coloured dot + label)
// directly above the chart area so the dimension reads from the
// chart itself.

describe("<LineChartCard>", () => {
    it("renders the seriesLabel as an inline legend when provided", () => {
        render(
            <LineChartCard
                title="Sessions"
                seriesLabel="Sessions per hour"
                data={[
                    { label: "00", value: 0 },
                    { label: "01", value: 5 },
                ]}
            />,
        );
        expect(screen.getByText("Sessions per hour")).toBeInTheDocument();
        // The legend lives inside an element tagged for tests so a
        // future "did the chart legend render?" assertion has a
        // single anchor rather than walking siblings.
        const legend = screen.getByTestId("chart-legend");
        expect(legend).toBeInTheDocument();
    });

    it("does NOT render a legend when seriesLabel is omitted", () => {
        render(
            <LineChartCard
                title="Sessions"
                data={[
                    { label: "00", value: 0 },
                    { label: "01", value: 5 },
                ]}
            />,
        );
        expect(screen.queryByTestId("chart-legend")).toBeNull();
    });
});
