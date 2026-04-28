import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { Outlet, RouterProvider, createMemoryRouter } from "react-router-dom";

// Step 1 of the settings reorg: Enrollment moves from a standalone
// /projects/<slug>/enrollment surface into the FLEET sub-tree. The
// canonical URL is now /projects/<slug>/fleet/enroll. The old URL
// continues to resolve, but only as a redirect to the new one — that
// keeps existing bookmarks, external docs, and the e2e fixtures
// working without freezing the URL shape.
//
// The actual route table lives in src/routes.tsx. To keep this spec
// independent of lazy-loaded chunks, we import the bare `routeTree`
// (the route-objects array) and mount a memory router from it. That
// way the test exercises the same data structure the production
// browser router uses, just without the network-y chunk fetches.

// Both layout wrappers (RequireAuth, ProjectShell) are stubbed to
// render <Outlet /> so the matched child route reaches the screen.
// React-router treats `element` slots as the layout for their
// `children`, and the layout MUST render an Outlet for nested
// routes to mount.
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
    return { router: r, ...render(<RouterProvider router={r} />) };
}

describe("enrollment routing — moved under /fleet/enroll", () => {
    it("renders EnrollmentPage at /projects/<slug>/fleet/enroll", async () => {
        renderAt("/projects/test-project/fleet/enroll");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
    });

    it("redirects the legacy /projects/<slug>/enrollment to /fleet/enroll", async () => {
        const { router } = renderAt("/projects/test-project/enrollment");
        // Wait for the redirect to settle — react-router runs the
        // <Navigate replace /> on the next tick after the route
        // matches, and the EnrollmentPage suspense boundary then
        // streams the page in.
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(
            "/projects/test-project/fleet/enroll",
        );
    });
});
