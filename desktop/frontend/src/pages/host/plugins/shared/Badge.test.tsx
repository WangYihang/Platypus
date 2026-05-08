import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { Badge } from "./Badge";
import { palette } from "../../../../layout/theme";

describe("<Badge>", () => {
    it("maps tone → background colour from the palette", () => {
        render(<Badge tone="success">running</Badge>);
        const el = screen.getByText("running");
        // jsdom serialises CSS colours as rgb(); check via the
        // data-tone attribute (stable, not subject to colour parsing)
        // and the inline style independently.
        expect(el.getAttribute("data-tone")).toBe("success");
        expect(el.style.background).toBe(rgb(palette.success));
    });

    it("shape=\"pill\" rounds to 999 (default); shape=\"tag\" rounds to 4", () => {
        const { rerender } = render(<Badge tone="muted">x</Badge>);
        expect(screen.getByText("x").style.borderRadius).toBe("999px");

        rerender(
            <Badge tone="muted" shape="tag">
                x
            </Badge>,
        );
        expect(screen.getByText("x").style.borderRadius).toBe("4px");
    });

    it("renders children verbatim (allows ReactNode, not just string)", () => {
        render(
            <Badge tone="warning">
                <span data-testid="inner">7</span>
            </Badge>,
        );
        expect(screen.getByTestId("inner")).toHaveTextContent("7");
    });
});

// Hex `#xxyyzz` → `rgb(x, y, z)` so we can compare against
// jsdom's normalised inline-style serialisation.
function rgb(hex: string): string {
    if (!hex.startsWith("#") || hex.length !== 7) {
        // Non-hex (e.g. rgba(...)) — return verbatim; the caller's
        // assertion has to match whatever jsdom emits.
        return hex;
    }
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return `rgb(${r}, ${g}, ${b})`;
}
