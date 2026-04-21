import { Navigate, createBrowserRouter } from "react-router-dom";
import { ConfigProvider } from "antd";

import { antTheme } from "./layout/theme";
import RequireAuth from "./layout/RequireAuth";
import ProjectShell from "./layout/ProjectShell";
import LoginRoute from "./routes/LoginRoute";
import ProjectsLanding from "./pages/ProjectsLanding";
import ProjectOverviewRoute from "./routes/ProjectOverviewRoute";
import HostsPage from "./pages/HostsPage";
import HostViewRoute from "./routes/HostViewRoute";
import ListenersPage from "./pages/ListenersPage";
import ListenerDetailPage from "./pages/ListenerDetailPage";
import SessionsPage from "./pages/SessionsPage";
import DispatchRoute from "./routes/DispatchRoute";
import MembersRoute from "./routes/MembersRoute";
import AdminUsers from "./pages/admin/AdminUsers";

// Top-level route table. The new flat-nav IA — every project-scoped
// view sits under /projects/:projectSlug/<page>, with shared sidebar
// chrome rendered by <ProjectShell>.
//
// HostsPage / SessionsPage are placeholders in step 3; ListenerView
// still serves both /listeners and /listeners/:listenerId via its
// existing dual-mode component (step 7 splits it).
export const router = createBrowserRouter([
    {
        path: "/login",
        element: (
            <ConfigProvider theme={antTheme}>
                <LoginRoute />
            </ConfigProvider>
        ),
    },
    {
        element: (
            <ConfigProvider theme={antTheme}>
                <RequireAuth />
            </ConfigProvider>
        ),
        children: [
            { path: "/", element: <Navigate to="/projects" replace /> },
            {
                element: <ProjectShell />,
                children: [
                    { path: "/projects", element: <ProjectsLanding /> },
                    { path: "/admin/users", element: <AdminUsers /> },
                ],
            },
            {
                path: "/projects/:projectSlug",
                element: <ProjectShell requireProject />,
                children: [
                    { index: true, element: <Navigate to="overview" replace /> },
                    { path: "overview", element: <ProjectOverviewRoute /> },
                    { path: "hosts", element: <HostsPage /> },
                    {
                        path: "hosts/:hostId",
                        element: <Navigate to="terminal" replace />,
                    },
                    { path: "hosts/:hostId/:tab", element: <HostViewRoute /> },
                    { path: "listeners", element: <ListenersPage /> },
                    { path: "listeners/:listenerId", element: <ListenerDetailPage /> },
                    { path: "sessions", element: <SessionsPage /> },
                    { path: "dispatch", element: <DispatchRoute /> },
                    { path: "members", element: <MembersRoute /> },
                ],
            },
            // Catch-all → projects landing.
            { path: "*", element: <Navigate to="/projects" replace /> },
        ],
    },
]);
