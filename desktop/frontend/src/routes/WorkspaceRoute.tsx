import { useNavigate } from "react-router-dom";

import Workspace from "../pages/Workspace";

// WorkspaceRoute is the temporary landing for all authenticated paths
// (set up in Step 2). Step 3 will replace it with a proper ProjectShell
// + nested route tree; for now it just delegates to the existing
// Workspace and wires its logout callback to the router.
//
// ProfileRail already calls lib/auth.logout() before invoking onLoggedOut,
// so we only need to navigate here.
export default function WorkspaceRoute() {
    const navigate = useNavigate();
    return <Workspace onLoggedOut={() => navigate("/login", { replace: true })} />;
}
