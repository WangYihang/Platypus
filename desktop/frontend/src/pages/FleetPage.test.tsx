import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
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

// FleetPage now reads `qk.hosts(project.id)` to derive the
// PageHeader pill counts (online / offline) and mounts the
// EnrollAgentWizard child (which imports a few install-related
// helpers). Mock everything the import graph touches so the test
// doesn't need a backend; useQuery surfaces missing-impl as `error`
// instead of throwing at render so we don't even need every helper
// to resolve, but listing them explicitly here keeps the failure
// mode obvious if the imports drift.
vi.mock("../lib/api", () => ({
    listHosts: vi.fn().mockResolvedValue([]),
    listInstallArtifacts: vi.fn().mockResolvedValue([]),
    listInstallPlatforms: vi.fn().mockResolvedValue({
        channel: "stable",
        platforms: [],
    }),
    issueInstallArtifact: vi.fn(),
    revokeInstallArtifact: vi.fn(),
    pendingApprovalCount: vi.fn().mockResolvedValue(0),
    getServerInfo: vi.fn().mockResolvedValue({ public_addr: "" }),
    approveHost: vi.fn(),
    rejectHost: vi.fn(),
}));

import FleetPage from "./FleetPage";
import { writePreference } from "../lib/preferences";
import { renderWithQueryClient } from "../testing/renderWithQueryClient";

// FleetPage's view ToggleGroup is now wrapped in a span carrying a
// title= attribute that names the user's stored default-view
// preference and points at /preferences as the place to change it.
// Without this hint, switching projects could land on an
// unexpected default with no clue why.

describe("<FleetPage>", () => {
    it("annotates the view toggle with the current default-view preference", () => {
        writePreference("ui.fleet.defaultView", "cards");
        const { container } = renderWithQueryClient(
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

    // 2026-04: enrollment is no longer a separate page — it's a
    // multi-step wizard mounted on top of Fleet itself, opened by
    // the URL search param `?enroll=1`. The header "Enroll agent"
    // entry point therefore links to `?enroll=1` (relative — keeps
    // whichever Fleet view is active) instead of routing away. This
    // spec pins the new wire format so a future refactor can't
    // accidentally re-route enrollment off-page.
    it("renders an 'Enroll agent' link in the header that opens the wizard via ?enroll=1", () => {
        renderWithQueryClient(
            <MemoryRouter initialEntries={["/projects/test-project/fleet"]}>
                <FleetPage />
            </MemoryRouter>,
        );
        const link = screen.getByRole("link", { name: /enroll agent/i });
        expect(link).toBeInTheDocument();
        // The href the browser sees may be relative or absolute
        // depending on how react-router resolves the link; either
        // way it must end with `enroll=1` so the wizard opens.
        const href = link.getAttribute("href") ?? "";
        expect(href).toMatch(/[?&]enroll=1\b/);
    });
});
