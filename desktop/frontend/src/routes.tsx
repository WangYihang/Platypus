import { ReactNode, Suspense, lazy } from "react";
import { Navigate, createBrowserRouter } from "react-router-dom";
import { Loader2 } from "lucide-react";

import { palette } from "./layout/theme";
import RequireAuth from "./layout/RequireAuth";
import ProjectShell from "./layout/ProjectShell";

// Every page component is code-split into its own chunk. The app shell
// (RequireAuth + ProjectShell + layout primitives) stays eagerly
// loaded — Suspense below paints a full-viewport spinner while the
// page chunk streams in, which on LAN / same-origin is a 50-200ms blip.
const LoginRoute = lazy(() => import("./routes/LoginRoute"));
const ProjectsLanding = lazy(() => import("./pages/ProjectsLanding"));
const ProjectOverviewRoute = lazy(() => import("./routes/ProjectOverviewRoute"));
const HostsPage = lazy(() => import("./pages/HostsPage"));
const HostViewRoute = lazy(() => import("./routes/HostViewRoute"));
const ListenersPage = lazy(() => import("./pages/ListenersPage"));
const ListenerDetailPage = lazy(() => import("./pages/ListenerDetailPage"));
const SessionsPage = lazy(() => import("./pages/SessionsPage"));
const ActivitiesPage = lazy(() => import("./pages/ActivitiesPage"));
const EnrollmentPage = lazy(() => import("./pages/EnrollmentPage"));
const DispatchRoute = lazy(() => import("./routes/DispatchRoute"));
const MembersRoute = lazy(() => import("./routes/MembersRoute"));
const AdminUsers = lazy(() => import("./pages/admin/AdminUsers"));
const TopologyPage = lazy(() => import("./pages/TopologyPage"));

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

// Top-level route table. Flat nav IA — every project-scoped view sits
// under /projects/:projectSlug/<page>, with shared sidebar chrome
// rendered by <ProjectShell>. Page components are lazy-loaded so the
// initial bundle only pulls in the shell + shadcn/ui primitives.
export const router = createBrowserRouter([
    {
        path: "/login",
        element: withSuspense(<LoginRoute />),
    },
    {
        element: <RequireAuth />,
        children: [
            { path: "/", element: <Navigate to="/projects" replace /> },
            {
                element: <ProjectShell />,
                children: [
                    { path: "/projects", element: withSuspense(<ProjectsLanding />) },
                    { path: "/admin/users", element: withSuspense(<AdminUsers />) },
                ],
            },
            {
                path: "/projects/:projectSlug",
                element: <ProjectShell requireProject />,
                children: [
                    { index: true, element: <Navigate to="overview" replace /> },
                    { path: "overview", element: withSuspense(<ProjectOverviewRoute />) },
                    { path: "hosts", element: withSuspense(<HostsPage />) },
                    {
                        path: "hosts/:hostId",
                        element: <Navigate to="terminal" replace />,
                    },
                    { path: "hosts/:hostId/:tab", element: withSuspense(<HostViewRoute />) },
                    { path: "listeners", element: withSuspense(<ListenersPage />) },
                    {
                        path: "listeners/:listenerId",
                        element: withSuspense(<ListenerDetailPage />),
                    },
                    { path: "sessions", element: withSuspense(<SessionsPage />) },
                    { path: "topology", element: withSuspense(<TopologyPage />) },
                    { path: "activities", element: withSuspense(<ActivitiesPage />) },
                    { path: "enrollment", element: withSuspense(<EnrollmentPage />) },
                    { path: "dispatch", element: withSuspense(<DispatchRoute />) },
                    { path: "members", element: withSuspense(<MembersRoute />) },
                ],
            },
            // Catch-all → projects landing.
            { path: "*", element: <Navigate to="/projects" replace /> },
        ],
    },
]);
