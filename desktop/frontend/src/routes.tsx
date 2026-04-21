import { Navigate, createBrowserRouter } from "react-router-dom";
import { ConfigProvider } from "antd";

import { antTheme } from "./layout/theme";
import RequireAuth from "./layout/RequireAuth";
import LoginRoute from "./routes/LoginRoute";
import WorkspaceRoute from "./routes/WorkspaceRoute";

// Top-level route table. Round 2 lays the track here; subsequent steps
// peel pages out of WorkspaceRoute into their own routes (HostsPage,
// ListenersPage, SessionsPage, etc.) under a ProjectShell outlet.
//
// All non-login paths sit under <RequireAuth>, which gates on the JWT
// session and redirects to /login if missing.
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
            // Catch-all for the workspace shell — Step 3 will replace this
            // with a proper route tree (/projects, /projects/:slug/*, etc.).
            { path: "*", element: <WorkspaceRoute /> },
        ],
    },
]);
