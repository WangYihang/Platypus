import { ReactNode, Suspense, lazy } from "react";
import {
    Navigate,
    RouteObject,
    createBrowserRouter,
    useRouteError,
} from "react-router-dom";
import { Loader2 } from "lucide-react";

import { font, palette, radius } from "./layout/theme";
import RequireAuth from "./layout/RequireAuth";
import ProjectShell from "./layout/ProjectShell";

// Every page component is code-split into its own chunk. The app shell
// (RequireAuth + ProjectShell + layout primitives) stays eagerly
// loaded — Suspense below paints a full-viewport spinner while the
// page chunk streams in, which on LAN / same-origin is a 50-200ms blip.
const LoginRoute = lazy(() => import("./routes/LoginRoute"));
const Onboarding = lazy(() => import("./pages/Onboarding"));
const ProjectsLanding = lazy(() => import("./pages/ProjectsLanding"));
const ProjectOverviewRoute = lazy(() => import("./routes/ProjectOverviewRoute"));
const HostsShell = lazy(() => import("./pages/HostsShell"));
const SecurityPage = lazy(() => import("./pages/SecurityPage"));
const HostsView = lazy(() => import("./pages/fleet/HostsView"));
const TopologyPanel = lazy(() => import("./pages/fleet/TopologyPanel"));
const HostViewRoute = lazy(() => import("./routes/HostViewRoute"));
const ActivityShell = lazy(() => import("./pages/ActivityShell"));
const SessionsPanel = lazy(() => import("./pages/fleet/SessionsPanel"));
const ActivitiesPage = lazy(() => import("./pages/ActivitiesPage"));
const RecordingsPage = lazy(() => import("./pages/RecordingsPage"));
const TransfersPage = lazy(() => import("./pages/TransfersPage"));
const EnrollmentPage = lazy(() => import("./pages/enrollment/EnrollmentPage"));
const MembersRoute = lazy(() => import("./routes/MembersRoute"));
const ProjectSettings = lazy(() => import("./pages/ProjectSettings"));
const Preferences = lazy(() => import("./pages/Preferences"));
const Account = lazy(() => import("./pages/Account"));
const AdminUsers = lazy(() => import("./pages/admin/AdminUsers"));
const MarketplacePage = lazy(() => import("./pages/MarketplacePage"));
const PluginDetailPage = lazy(() => import("./pages/marketplace/PluginDetailPage"));
const AdminSettings = lazy(() => import("./pages/admin/AdminSettings"));
const AdminAccessControl = lazy(() => import("./pages/admin/AdminAccessControl"));
const AdminLayout = lazy(() => import("./pages/admin/AdminLayout"));
const Servers = lazy(() => import("./pages/Servers"));

// routeFallback is the placeholder each lazy route renders while its
// chunk is fetched. Centred spinner over the main surface so it doesn't
// jank around the sidebar chrome that's already mounted.
function routeFallback(): ReactNode {
    return (
        <div
            style={{
                display: "flex",
                justifyContent: "center",
                alignItems: "center",
                width: "100%",
                height: "100%",
                minHeight: 200,
                background: palette.main,
            }}
        >
            <Loader2 className="size-5 animate-spin text-text-muted" />
        </div>
    );
}

// withSuspense wraps a lazy element in a Suspense boundary. Centralising
// the wrapper keeps the route table readable and guarantees every
// code-split page renders the same fallback.
function withSuspense(element: ReactNode): ReactNode {
    return <Suspense fallback={routeFallback()}>{element}</Suspense>;
}

// RootErrorBoundary catches rendering and chunk-loading errors.
// Specifically, it detects "Failed to fetch dynamically imported module"
// (which happens when a new version is deployed and old chunks are gone)
// and reloads the page once to pull the fresh index.html.
function RootErrorBoundary() {
    const error = useRouteError() as Error;

    const isChunkLoadError =
        error?.message?.includes("Failed to fetch dynamically imported module") ||
        error?.message?.includes("Importing a module script failed");

    if (isChunkLoadError) {
        const hasReloaded = sessionStorage.getItem("app-reloaded-on-error");
        if (!hasReloaded) {
            sessionStorage.setItem("app-reloaded-on-error", "true");
            window.location.reload();
            return null;
        }
    }

    // Clear reload flag if we either successfully reloaded (and now have a
    // different error) or if this isn't a chunk error at all.
    sessionStorage.removeItem("app-reloaded-on-error");

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                height: "100vh",
                width: "100vw",
                background: palette.main,
                color: palette.textPrimary,
                padding: "2rem",
                textAlign: "center",
                fontFamily: font.mono,
            }}
        >
            <h1 style={{ marginBottom: "1rem", fontSize: "1.5rem", fontWeight: "bold" }}>
                Unexpected Application Error
            </h1>
            <p style={{ marginBottom: "2rem", color: palette.textMuted, maxWidth: "600px" }}>
                {error?.message || "An unknown error occurred while rendering this page."}
            </p>
            <button
                style={{
                    padding: "0.5rem 1rem",
                    borderRadius: radius.md,
                    background: palette.accent,
                    color: palette.accentFg,
                    border: "none",
                    cursor: "pointer",
                    fontWeight: 500,
                }}
                onClick={() => window.location.reload()}
            >
                Reload Page
            </button>
        </div>
    );
}

// Top-level route table. Each project-scoped resource maps cleanly to
// either "fleet inventory" (hosts), "rollup of fleet child resources"
// (activity), or "project-global concern" (security / members /
// settings). See the IA decision log in CLAUDE.md.
//
// Exported as data so tests (src/routes/enrollmentRoute.test.tsx) can
// mount a memory router from the same source production uses, instead
// of duplicating the topology.
export const routeTree: RouteObject[] = [
    {
        path: "/login",
        element: withSuspense(<LoginRoute />),
        errorElement: <RootErrorBoundary />,
    },
    {
        path: "/onboarding",
        element: withSuspense(<Onboarding />),
        errorElement: <RootErrorBoundary />,
    },
    {
        element: <RequireAuth />,
        errorElement: <RootErrorBoundary />,
        children: [
            { path: "/", element: <Navigate to="/projects" replace /> },
            {
                element: <ProjectShell />,
                children: [
                    { path: "/projects", element: withSuspense(<ProjectsLanding />) },
                    { path: "/servers", element: withSuspense(<Servers />) },
                    // /admin gets a sub-tab strip via AdminLayout so
                    // Users / Access Control / Settings share chrome
                    // and the Admin top-tab can deep-link into any
                    // child without each page replicating the strip.
                    { path: "/admin", element: <Navigate to="/admin/users" replace /> },
                    {
                        path: "/admin",
                        element: withSuspense(<AdminLayout />),
                        children: [
                            { path: "users", element: withSuspense(<AdminUsers />) },
                            { path: "access-control", element: withSuspense(<AdminAccessControl />) },
                            { path: "settings", element: withSuspense(<AdminSettings />) },
                        ],
                    },
                    { path: "/account", element: withSuspense(<Account />) },
                    { path: "/preferences", element: withSuspense(<Preferences />) },
                    { path: "/marketplace", element: withSuspense(<MarketplacePage />) },
                    {
                        path: "/marketplace/plugins/:pluginID",
                        element: withSuspense(<PluginDetailPage />),
                    },
                ],
            },
            {
                path: "/projects/:projectSlug",
                element: <ProjectShell requireProject />,
                children: [
                    { index: true, element: <Navigate to="overview" replace /> },
                    { path: "overview", element: withSuspense(<ProjectOverviewRoute />) },
                    // Hosts is the fleet inventory plus master-detail
                    // host inspection. Sub-views: List (default), the
                    // network Topology graph, and per-host detail at
                    // hosts/:id/:tab. The host-scoped terminal drawer
                    // is mounted by HostsShell so it only appears
                    // here, not on /activity, /security, etc.
                    {
                        path: "hosts",
                        element: withSuspense(<HostsShell />),
                        children: [
                            { index: true, element: withSuspense(<HostsView />) },
                            { path: "topology", element: withSuspense(<TopologyPanel />) },
                            // hosts/:hostId without an activity lands on
                            // `files` — the VS-Code-style HostView treats
                            // the file browser as the centrepiece.
                            {
                                path: ":hostId",
                                element: <Navigate to="files" replace />,
                            },
                            {
                                path: ":hostId/:tab",
                                element: withSuspense(<HostViewRoute />),
                            },
                        ],
                    },
                    // Activity is the project-wide rollup of every
                    // per-host time-series resource. Each sub-tab is
                    // the same data the matching per-host tab surfaces,
                    // unioned across the fleet.
                    {
                        path: "activity",
                        element: withSuspense(<ActivityShell />),
                        children: [
                            { index: true, element: <Navigate to="sessions" replace /> },
                            { path: "sessions", element: withSuspense(<SessionsPanel />) },
                            { path: "events", element: withSuspense(<ActivitiesPage />) },
                            { path: "recordings", element: withSuspense(<RecordingsPage />) },
                            { path: "transfers", element: withSuspense(<TransfersPage />) },
                        ],
                    },
                    { path: "security", element: withSuspense(<SecurityPage />) },
                    // Enrollment is the agent-onboarding hub: install
                    // commands, raw enrollment tokens, and the
                    // approval queue for fresh agents waiting to join.
                    // The page reads :tab from the URL so deep links
                    // to /enrollment/approvals work from anywhere.
                    {
                        path: "enrollment",
                        children: [
                            { index: true, element: withSuspense(<EnrollmentPage />) },
                            { path: ":tab", element: withSuspense(<EnrollmentPage />) },
                        ],
                    },
                    { path: "members", element: withSuspense(<MembersRoute />) },
                    { path: "settings", element: withSuspense(<ProjectSettings />) },
                ],
            },
            // Catch-all → projects landing.
            { path: "*", element: <Navigate to="/projects" replace /> },
        ],
    },
];

export const router = createBrowserRouter(routeTree);
