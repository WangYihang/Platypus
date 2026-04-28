import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("../layout/ProjectShell", () => ({
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

vi.mock("./fleet/HostsCardPanel", () => ({ default: () => <div /> }));
vi.mock("./fleet/HostsPanel", () => ({ default: () => <div /> }));
vi.mock("./fleet/SessionsPanel", () => ({ default: () => <div /> }));
vi.mock("./fleet/TopologyPanel", () => ({ default: () => <div /> }));
vi.mock("../components/EnrollmentWaitBanner", () => ({ default: () => null }));

import FleetPage from "./FleetPage";
import { writePreference } from "../lib/preferences";

// FleetPage's view ToggleGroup is now wrapped in a span carrying a
// title= attribute that names the user's stored default-view
// preference and points at /preferences as the place to change it.
// Without this hint, switching projects could land on an
// unexpected default with no clue why.

describe("<FleetPage>", () => {
    it("annotates the view toggle with the current default-view preference", () => {
        writePreference("ui.fleet.defaultView", "cards");
        const { container } = render(
            <MemoryRouter>
                <FleetPage />
            </MemoryRouter>,
        );
        const toggle = container.querySelector(
            '[data-testid="fleet-view-toggle"]',
        );
        expect(toggle).not.toBeNull();
        const title = toggle!.getAttribute("title") ?? "";
        expect(title).toMatch(/default view/i);
        expect(title).toMatch(/cards/i);
        expect(title).toMatch(/preferences/i);
    });

    // Step 1 of the settings reorg pulls Enrollment under Fleet. The
    // sidebar no longer carries a top-level "Enrollment" link — the
    // way to add a host now is from inside Fleet itself. So the page
    // header has to surface a clearly-labelled "Enroll agent"
    // entry point that takes the user to /fleet/enroll, otherwise
    // there's no discoverable path to onboarding once the sidebar
    // item is gone.
    it("renders an 'Enroll agent' link in the header pointing at /fleet/enroll", () => {
        render(
            <MemoryRouter>
                <FleetPage />
            </MemoryRouter>,
        );
        const link = screen.getByRole("link", { name: /enroll agent/i });
        expect(link).toBeInTheDocument();
        expect(link.getAttribute("href")).toBe(
            "/projects/test-project/fleet/enroll",
        );
    });
});
