import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { Outlet, RouterProvider, createMemoryRouter } from "react-router-dom";

import { renderWithQueryClient } from "../testing/renderWithQueryClient";

// Enrollment management lives under the Operations hub (write-capable
// runtime state). Day-to-day enrollment still happens through the
// EnrollAgentWizard dialog (URL param `?enroll=1` on /fleet); this
// page that lists historical install artifacts and tokens — rarely
// visited — sits at /projects/<slug>/operations/enrollment.
//
// Legacy URLs keep resolving via redirects to the new canonical path
// so existing bookmarks, docs, and e2e fixtures continue to land
// somewhere sensible:
//
//   /projects/<slug>/enrollment            → operations/enrollment
//   /projects/<slug>/fleet/enroll          → operations/enrollment
//   /projects/<slug>/audit/enrollment      → operations/enrollment
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

const CANONICAL = "/projects/test-project/operations/enrollment";

describe("enrollment routing — moved under /operations/enrollment", () => {
    it("renders EnrollmentPage at the canonical /operations/enrollment path", async () => {
        const { router } = renderAt(CANONICAL);
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });

    it("redirects legacy /projects/<slug>/enrollment to /operations/enrollment", async () => {
        const { router } = renderAt("/projects/test-project/enrollment");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });

    it("redirects legacy /projects/<slug>/fleet/enroll to /operations/enrollment", async () => {
        const { router } = renderAt("/projects/test-project/fleet/enroll");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });

    it("redirects legacy /projects/<slug>/audit/enrollment to /operations/enrollment", async () => {
        const { router } = renderAt("/projects/test-project/audit/enrollment");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });
});
