import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";

import SecurityBadge from "./SecurityBadge";

// SecurityBadge has three states keyed off the host's latest scan
// summary:
//   1. counts == null/undefined  → render nothing (host never scanned)
//   2. counts present, all zero  → small green check (clean)
//   3. counts present, any > 0   → coloured pill, label = worst severity
// The "never scanned" case must be visually identical to pre-feature
// builds so adopting persistence doesn't make every existing card
// sprout a placeholder badge.

describe("<SecurityBadge>", () => {
    it("renders nothing when counts are absent (never scanned)", () => {
        const { container } = render(<SecurityBadge counts={null} />);
        expect(container).toBeEmptyDOMElement();
    });

    it("renders a 'clean' check when scanned with all-zero counts", () => {
        const { container, queryByText } = render(
            <SecurityBadge
                counts={{ critical: 0, high: 0, medium: 0, low: 0, info: 0 }}
                scannedAtUnix={1_700_000_000}
            />,
        );
        // No textual "Crit"/"High" pill — just the icon-only span.
        expect(queryByText(/Crit|High|Med|Low/i)).toBeNull();
        // The component still rendered something (the green check).
        expect(container.firstChild).not.toBeNull();
    });

    it("highlights the worst severity present (critical wins over high)", () => {
        const { getByText } = render(
            <SecurityBadge
                counts={{ critical: 2, high: 5, medium: 0, low: 0, info: 0 }}
                compact={false}
            />,
        );
        expect(getByText(/Crit\s*2/)).toBeInTheDocument();
    });

    it("falls back to high when no critical findings", () => {
        const { getByText } = render(
            <SecurityBadge
                counts={{ critical: 0, high: 3, medium: 1, low: 0, info: 0 }}
                compact={false}
            />,
        );
        expect(getByText(/High\s*3/)).toBeInTheDocument();
    });

    it("compact mode renders just the count, no label text", () => {
        const { getByText, queryByText } = render(
            <SecurityBadge
                counts={{ critical: 7, high: 0, medium: 0, low: 0, info: 0 }}
                compact
            />,
        );
        expect(getByText("7")).toBeInTheDocument();
        expect(queryByText(/Crit/)).toBeNull();
    });
});
