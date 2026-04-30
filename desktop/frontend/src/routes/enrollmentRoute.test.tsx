import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { Outlet, RouterProvider, createMemoryRouter } from "react-router-dom";

import { renderWithQueryClient } from "../testing/renderWithQueryClient";

// EnrollmentPage is the agent-onboarding hub mounted at
// /projects/<slug>/enrollment. Three sub-tabs (install commands /
// enrollment tokens / approvals) live under :tab; the page reads the
// param so deep links like /enrollment/approvals open straight to the
// approvals queue. This spec pins both behaviours.
//
// The actual route table lives in src/routes.tsx. To keep this spec
// independent of lazy-loaded chunks, we import the bare `routeTree`
// (the route-objects array) and mount a memory router from it.

vi.mock("../layout/ProjectShell", () => ({
    default: () => <Outlet />,
    useCurrentProject: () => ({
        id: "p1",
        slug: "test-project",
        name: "Test Project",
    }),
}));

vi.mock("../layout/RequireAuth", () => ({
    default: () => <Outlet />,
}));

vi.mock("../lib/api", () => ({
    listInstallArtifacts: vi.fn().mockResolvedValue([]),
    listInstallPlatforms: vi.fn().mockResolvedValue({
        channel: "stable",
        platforms: [{ os: "linux", arch: "amd64" }],
    }),
    listEnrollmentTokens: vi.fn().mockResolvedValue([]),
    listPendingApprovals: vi.fn().mockResolvedValue([]),
    pendingApprovalCount: vi.fn().mockResolvedValue(0),
    issueInstallArtifact: vi.fn(),
    issueEnrollmentToken: vi.fn(),
    revokeInstallArtifact: vi.fn(),
    revokeEnrollmentToken: vi.fn(),
    getServerInfo: vi.fn().mockResolvedValue({
        server_endpoint: "127.0.0.1:7332",
        version: "test",
        commit: "test",
    }),
}));

import { routeTree } from "../routes";

function renderAt(path: string) {
    const r = createMemoryRouter(routeTree, { initialEntries: [path] });
    return { router: r, ...renderWithQueryClient(<RouterProvider router={r} />) };
}

describe("enrollment routing", () => {
    it("renders EnrollmentPage at /projects/<slug>/enrollment with install tab default", async () => {
        const { router } = renderAt("/projects/test-project/enrollment");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(
            "/projects/test-project/enrollment",
        );
    });

    it("deep-links to the approvals tab via /enrollment/approvals", async () => {
        renderAt("/projects/test-project/enrollment/approvals");
        const approvalsTab = await screen.findByRole("tab", { name: /approvals/i });
        expect(approvalsTab).toBeInTheDocument();
        expect(approvalsTab.getAttribute("data-state")).toBe("active");
    });

    it("deep-links to the tokens tab via /enrollment/tokens", async () => {
        renderAt("/projects/test-project/enrollment/tokens");
        const tokensTab = await screen.findByRole("tab", {
            name: /enrollment tokens/i,
        });
        expect(tokensTab.getAttribute("data-state")).toBe("active");
    });
});
