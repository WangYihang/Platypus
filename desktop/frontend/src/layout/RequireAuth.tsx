import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Loader2 } from "lucide-react";

import { getSession, onSessionChange, refresh } from "../lib/auth";
import { getActiveServer, listServers } from "../lib/servers";

// RequireAuth gates the protected route subtree on having a valid JWT
// session. On first mount we try to rehydrate from the active server's
// persisted refresh token (browser reload, app relaunch). Until that
// resolves we show a spinner so the UI doesn't flash the login page.
//
// Three terminal states:
//   · session present → render <Outlet />
//   · no session, at least one server saved → /login (with the active
//     profile's URL pre-filled via state so the user just types the
//     password)
//   · no session, no servers saved → /onboarding (the three-step
//     welcome wizard)
export default function RequireAuth() {
    const [ready, setReady] = useState(false);
    const [hasSession, setHasSession] = useState(false);
    const location = useLocation();

    useEffect(() => {
        (async () => {
            if (getSession()) {
                setHasSession(true);
            } else {
                setHasSession(await refresh());
            }
            setReady(true);
        })();
    }, []);

    useEffect(
        () =>
            onSessionChange(() => {
                setHasSession(!!getSession());
            }),
        [],
    );

    if (!ready) {
        return (
            <div className="flex items-center justify-center p-20">
                <Loader2 className="size-6 animate-spin text-text-muted" />
            </div>
        );
    }
    if (!hasSession) {
        if (listServers().length === 0) {
            return <Navigate to="/onboarding" replace state={{ from: location }} />;
        }
        const active = getActiveServer();
        return (
            <Navigate
                to="/login"
                replace
                state={{
                    from: location,
                    serverId: active?.id,
                    serverURL: active?.url,
                }}
            />
        );
    }
    return <Outlet />;
}
