import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spin } from "antd";

import { getSession, onSessionChange, refresh } from "../lib/auth";

// RequireAuth gates the protected route subtree on having a valid JWT
// session. On first mount it tries to rehydrate from the persisted
// refresh token (browser reload, app relaunch). Until that resolves we
// show a spinner so the UI doesn't flash the login page.
//
// When the session disappears (logout, expired refresh) we redirect to
// /login and stash the current location so post-login can bounce back.
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

    useEffect(() =>
        onSessionChange(() => {
            setHasSession(!!getSession());
        }),
    []);

    if (!ready) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin size="large" />
            </div>
        );
    }
    if (!hasSession) {
        return <Navigate to="/login" replace state={{ from: location }} />;
    }
    return <Outlet />;
}
