import { ReactNode, Suspense, lazy } from "react";
import {
    Navigate,
    RouteObject,
    createBrowserRouter,
    useParams,
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
const FleetPage = lazy(() => import("./pages/FleetPage"));
const SecurityPage = lazy(() => import("./pages/SecurityPage"));
const HostsView = lazy(() => import("./pages/fleet/HostsView"));
const SessionsPanel = lazy(() => import("./pages/fleet/SessionsPanel"));
const TopologyPanel = lazy(() => import("./pages/fleet/TopologyPanel"));
const HostViewRoute = lazy(() => import("./routes/HostViewRoute"));
const HistoryPage = lazy(() => import("./pages/HistoryPage"));
const OperationsPage = lazy(() => import("./pages/OperationsPage"));
const ActivitiesPage = lazy(() => import("./pages/ActivitiesPage"));
const RecordingsPage = lazy(() => import("./pages/RecordingsPage"));
const TransfersPage = lazy(() => import("./pages/TransfersPage"));
const EnrollmentPage = lazy(() => import("./pages/enrollment/EnrollmentPage"));
const ApprovalsPage = lazy(() => import("./pages/ApprovalsPage"));
const MembersRoute = lazy(() => import("./routes/MembersRoute"));
const ProjectSettings = lazy(() => import("./pages/ProjectSettings"));
const Preferences = lazy(() => import("./pages/Preferences"));
const Account = lazy(() => import("./pages/Account"));
const AdminUsers = lazy(() => import("./pages/admin/AdminUsers"));
const AdminSettings = lazy(() => import("./pages/admin/AdminSettings"));
const AdminAccessControl = lazy(() => import("./pages/admin/AdminAccessControl"));
const AdminLayout = lazy(() => import("./pages/admin/AdminLayout"));
const Servers = lazy(() => import("./pages/Servers"));

// LegacyHostRedirect bridges old `/projects/<slug>/hosts/<id>(/<tab>)`
// URLs to the new master-detail path under `/fleet/hosts`. We can't
// use a static <Navigate to=…> because the target needs `:hostId` /
// `:tab` interpolated; reading useParams() gives us the current
// values to splice into the redirect target.
function LegacyHostRedirect() {
    const params = useParams<{
        projectSlug: string;
        hostId: string;
        tab?: string;
    }>();
    const tab = params.tab ?? "files";
    return (
        <Navigate
            to={`/projects/${params.projectSlug}/fleet/hosts/${params.hostId}/${tab}`}
            replace
        />
    );
}

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

// Top-level route table. Flat nav IA — every project-scoped view sits
// under /projects/:projectSlug/<page>, with shared sidebar chrome
// rendered by <ProjectShell>. Page components are lazy-loaded so the
// initial bundle only pulls in the shell + shadcn/ui primitives.
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
                ],
            },
            {
                path: "/projects/:projectSlug",
                element: <ProjectShell requireProject />,
                children: [
                    { index: true, element: <Navigate to="overview" replace /> },
                    { path: "overview", element: withSuspense(<ProjectOverviewRoute />) },
                    { path: "security", element: withSuspense(<SecurityPage />) },
                    // Fleet is the parent route for hosts / sessions /
                    // topology / approvals. The four legacy panels
                    // mounted via `?view=` got broken out into proper
                    // sub-routes so URL = activity (deep-linkable,
                    // bookmarkable, browser-back works), and the
                    // master-detail HostView lives under hosts/:id/:activity.
                    {
                        path: "fleet",
                        element: withSuspense(<FleetPage />),
                        children: [
                            { index: true, element: <Navigate to="hosts" replace /> },
                            {
                                path: "hosts",
                                element: withSuspense(<HostsView />),
                                children: [
                                    // hosts/:hostId without an activity
                                    // lands on `files` — the VS-Code-style
                                    // HostView treats the file browser as
                                    // the centrepiece.
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
                            { path: "sessions", element: withSuspense(<SessionsPanel />) },
                            { path: "topology", element: withSuspense(<TopologyPanel />) },
                            { path: "approvals", element: withSuspense(<ApprovalsPage />) },
                            // Backwards-compat: /fleet/enroll used to
                            // be the standalone enrollment page. The
                            // canonical management surface (browse /
                            // revoke past artifacts + tokens) now
                            // lives under /operations/enrollment.
                            // Day-to-day enrollment is the
                            // EnrollAgentWizard mounted on FleetPage
                            // via the `?enroll=1` URL param.
                            {
                                path: "enroll",
                                element: (
                                    <Navigate
                                        to="../../operations/enrollment"
                                        replace
                                        relative="path"
                                    />
                                ),
                            },
                        ],
                    },
                    // Backwards-compat: hosts used to live as a flat
                    // sibling of /fleet. Master-detail moved them
                    // under /fleet/hosts so the surrounding chrome
                    // (sub-tabs, count pills) stays in place when
                    // jumping between hosts. Old deep links keep
                    // resolving via the LegacyHostRedirect helper
                    // that injects :hostId / :tab into the new path.
                    {
                        path: "hosts/:hostId",
                        element: <LegacyHostRedirect />,
                    },
                    {
                        path: "hosts/:hostId/:tab",
                        element: <LegacyHostRedirect />,
                    },
                    // History is the read-only audit hub: Activities
                    // (event log) + Recordings (session playback).
                    // Sister surface Operations owns the write-capable
                    // runtime state (Transfers, Enrollment).
                    {
                        path: "history",
                        element: withSuspense(<HistoryPage />),
                        children: [
                            { index: true, element: <Navigate to="activities" replace /> },
                            { path: "activities", element: withSuspense(<ActivitiesPage />) },
                            { path: "recordings", element: withSuspense(<RecordingsPage />) },
                        ],
                    },
                    // Operations is the write-capable runtime state hub
                    // — Transfers (in-flight uploads/downloads) and
                    // Enrollment (token + install-artifact management).
                    {
                        path: "operations",
                        element: withSuspense(<OperationsPage />),
                        children: [
                            { index: true, element: <Navigate to="transfers" replace /> },
                            { path: "transfers", element: withSuspense(<TransfersPage />) },
                            { path: "enrollment", element: withSuspense(<EnrollmentPage />) },
                        ],
                    },
                    // Backwards-compat redirects: the Audit hub used to
                    // bundle all four surfaces under one parent route.
                    // Old URLs / bookmarks still resolve to whichever
                    // hub now owns each surface. Each route is a flat
                    // sibling of `/projects/:slug` so a single `..`
                    // ascends to the project root.
                    { path: "audit", element: <Navigate to="../history/activities" replace /> },
                    {
                        path: "audit/activities",
                        element: <Navigate to="../history/activities" replace />,
                    },
                    {
                        path: "audit/recordings",
                        element: <Navigate to="../history/recordings" replace />,
                    },
                    {
                        path: "audit/transfers",
                        element: <Navigate to="../operations/transfers" replace />,
                    },
                    {
                        path: "audit/enrollment",
                        element: <Navigate to="../operations/enrollment" replace />,
                    },
                    {
                        path: "enrollment",
                        element: <Navigate to="../operations/enrollment" replace />,
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
