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

vi.mock("./fleet/enroll/EnrollAgentWizard", () => ({ default: () => null }));
vi.mock("../components/EnrollmentWaitBanner", () => ({ default: () => null }));

// FleetPage is now the parent route — it owns the Fleet title, the
// Enroll-agent button, and the sub-tab strip (Hosts · Sessions ·
// Topology · Approvals). Sub-pages render through <Outlet />, so
// the things this test pins are the chrome bits that always show
// regardless of which sub-tab is active.
//
// The Cards / Table view toggle that used to live here moved into
// HostsView (the Hosts sub-tab body) — see HostsView.test.tsx.
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
import { renderWithQueryClient } from "../testing/renderWithQueryClient";

describe("<FleetPage>", () => {
    it("renders the Fleet sub-tab strip (hosts · sessions · topology · approvals)", () => {
        renderWithQueryClient(
            <MemoryRouter initialEntries={["/projects/test-project/fleet/hosts"]}>
                <FleetPage />
            </MemoryRouter>,
        );
        const strip = screen.getByTestId("fleet-subtabs");
        expect(strip).toBeInTheDocument();
        expect(strip.textContent?.toLowerCase() ?? "").toContain("hosts");
        expect(strip.textContent?.toLowerCase() ?? "").toContain("sessions");
        expect(strip.textContent?.toLowerCase() ?? "").toContain("topology");
        expect(strip.textContent?.toLowerCase() ?? "").toContain("approvals");
    });

    // 2026-04: enrollment is no longer a separate page — it's a
    // multi-step wizard mounted on top of Fleet itself, opened by
    // the URL search param `?enroll=1`. The header "Enroll agent"
    // entry point therefore links to `?enroll=1` (relative — keeps
    // whichever Fleet sub-tab is active) instead of routing away. This
    // spec pins the new wire format so a future refactor can't
    // accidentally re-route enrollment off-page.
    it("renders an 'Enroll agent' link in the header that opens the wizard via ?enroll=1", () => {
        renderWithQueryClient(
            <MemoryRouter initialEntries={["/projects/test-project/fleet/hosts"]}>
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
