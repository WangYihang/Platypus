import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";

import Sparkline from "./Sparkline";

describe("<Sparkline>", () => {
    it("renders a flat baseline when given no samples", () => {
        const { container } = render(<Sparkline values={[]} title="empty" />);
        const svg = container.querySelector('[data-testid="sparkline"]');
        expect(svg).not.toBeNull();
        // Empty series falls back to a baseline <line>, not a polyline.
        expect(container.querySelector("polyline")).toBeNull();
        expect(container.querySelector("line")).not.toBeNull();
    });

    it("plots a polyline with one point per sample", () => {
        const { container } = render(<Sparkline values={[1, 2, 3, 4]} />);
        const poly = container.querySelector("polyline");
        expect(poly).not.toBeNull();
        const pts = poly!.getAttribute("points") ?? "";
        // One coord pair per sample, space separated.
        expect(pts.split(" ").filter(Boolean)).toHaveLength(4);
    });

    it("places the lowest value near the bottom edge and highest near the top", () => {
        const { container } = render(
            <Sparkline values={[0, 100]} width={40} height={20} />,
        );
        const poly = container.querySelector("polyline")!;
        const [p1, p2] = poly.getAttribute("points")!.split(" ");
        const y1 = Number(p1.split(",")[1]);
        const y2 = Number(p2.split(",")[1]);
        // First sample is the minimum → drawn lower (larger y);
        // second sample is the maximum → drawn higher (smaller y).
        expect(y1).toBeGreaterThan(y2);
    });
});
