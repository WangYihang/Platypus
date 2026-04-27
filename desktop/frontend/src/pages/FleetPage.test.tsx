import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
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
});
