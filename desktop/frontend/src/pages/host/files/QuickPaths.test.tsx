import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import QuickPaths from "./QuickPaths";

// QuickPaths is the chip-row component that lives just above the
// breadcrumb in FileBrowser. It consumes a Host (so it can render
// platform-appropriate chips) and an onSelect callback that
// FileBrowser uses to drive `dir.cd()`.
//
// The contract pinned here is purely UI:
//   1. Chips render in the order quickPathsForHost provides.
//   2. Clicking a chip fires onSelect with the chip's path.
//   3. With a null host (still loading) the row hides entirely so
//      it doesn't pop in mid-render.
//   4. Each chip carries a title= attribute for its tooltip context.

const linuxHost = {
    id: "h1",
    project_id: "p1",
    fingerprint: "fp",
    fingerprint_fallback: false,
    first_seen_at: "",
    last_seen_at: "",
    platform: "ubuntu",
    current_user: "alice",
};

describe("<QuickPaths>", () => {
    it("renders nothing when the host is still loading", () => {
        const { container } = render(
            <QuickPaths host={null} onSelect={vi.fn()} />,
        );
        expect(container.querySelector('[data-testid="files-quick-paths"]')).toBeNull();
    });

    it("renders the canonical Unix chip set for a Linux host", () => {
        render(<QuickPaths host={linuxHost as never} onSelect={vi.fn()} />);
        expect(screen.getByRole("button", { name: "/" })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: "~" })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: "/etc" })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: "/var" })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: "/tmp" })).toBeInTheDocument();
    });

    it("calls onSelect with the chip's path on click", () => {
        const onSelect = vi.fn();
        render(<QuickPaths host={linuxHost as never} onSelect={onSelect} />);

        fireEvent.click(screen.getByRole("button", { name: "/etc" }));
        expect(onSelect).toHaveBeenLastCalledWith("/etc");

        fireEvent.click(screen.getByRole("button", { name: "~" }));
        expect(onSelect).toHaveBeenLastCalledWith("/home/alice");
    });

    it("attaches a tooltip title to every chip", () => {
        render(<QuickPaths host={linuxHost as never} onSelect={vi.fn()} />);
        for (const label of ["/", "~", "/etc", "/var", "/tmp"]) {
            const btn = screen.getByRole("button", { name: label });
            expect(btn.getAttribute("title")?.length).toBeGreaterThan(0);
        }
    });
});
