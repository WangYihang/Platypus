import { useNavigate, useLocation } from "react-router-dom";

import Login from "../pages/Login";

interface LocationState {
    from?: { pathname: string };
    // When the rail sends the user here to re-auth a signed-out
    // server, it passes the profile id (and URL for display) via
    // location.state so we can pin the form to that server.
    serverId?: string;
    serverURL?: string;
}

// LoginRoute wraps the Login page with router-aware navigation. After a
// successful login we send the user to wherever they were trying to go
// (RequireAuth stashed it in location.state.from), or default to /projects.
export default function LoginRoute() {
    const navigate = useNavigate();
    const location = useLocation();
    const state = location.state as LocationState | null;
    const from = state?.from?.pathname || "/projects";

    return (
        <Login
            onLoggedIn={() => navigate(from, { replace: true })}
            pinnedServerId={state?.serverId}
            initialURL={state?.serverURL}
        />
    );
}
