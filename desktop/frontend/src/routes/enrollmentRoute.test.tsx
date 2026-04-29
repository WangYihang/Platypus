import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { Outlet, RouterProvider, createMemoryRouter } from "react-router-dom";

import { renderWithQueryClient } from "../testing/renderWithQueryClient";

// 2026-04 enrollment IA pass moves the management surface into the
// AUDIT sub-tree. Day-to-day enrollment now happens through the
// EnrollAgentWizard dialog (URL param `?enroll=1` on /fleet); the
// page that lists historical install artifacts and tokens — rarely
// visited — lives at /projects/<slug>/audit/enrollment.
//
// Both legacy URLs (the original /enrollment and the brief
// /fleet/enroll experiment) keep resolving as redirects to the new
// canonical path so existing bookmarks, docs, and e2e fixtures
// continue to land somewhere sensible.
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
    return { router: r, ...renderWithQueryClient(<RouterProvider router={r} />) };
}

const CANONICAL = "/projects/test-project/audit/enrollment";

describe("enrollment routing — moved under /audit/enrollment", () => {
    it("renders EnrollmentPage at the canonical /audit/enrollment path", async () => {
        const { router } = renderAt(CANONICAL);
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });

    it("redirects legacy /projects/<slug>/enrollment to /audit/enrollment", async () => {
        const { router } = renderAt("/projects/test-project/enrollment");
        // Wait for the redirect + AuditPage suspense to settle, then
        // the EnrollmentPage tab strip becomes reachable.
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });

    it("redirects legacy /projects/<slug>/fleet/enroll to /audit/enrollment", async () => {
        const { router } = renderAt("/projects/test-project/fleet/enroll");
        expect(
            await screen.findByRole("tab", { name: /install commands/i }),
        ).toBeInTheDocument();
        expect(router.state.location.pathname).toBe(CANONICAL);
    });
});
