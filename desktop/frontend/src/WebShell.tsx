import { useCallback, useEffect, useState } from "react";
import { ConfigProvider, Spin } from "antd";

import Login from "./pages/Login";
import Workspace from "./pages/Workspace";
import { antTheme } from "./layout/theme";
import { getSession, onSessionChange, refresh } from "./lib/auth";

// WebShell is the web-mode root. Gates on the new JWT session from
// lib/auth (access token in memory, refresh token in localStorage):
//
//  - No session      → <Login> — user signs in or bootstraps.
//  - Session present → <Workspace> — Slack-style sidebar UI.
//
// On first mount we try refresh() to rehydrate a session from a
// persisted refresh token (browser reload, app relaunch). Until that
// finishes we show a spinner so the UI doesn't flash the login card.
export default function WebShell() {
    const [ready, setReady] = useState(false);
    const [hasSession, setHasSession] = useState(false);

    useEffect(() => {
        (async () => {
            const ok = await refresh();
            setHasSession(ok);
            setReady(true);
        })();
    }, []);

    useEffect(() =>
        onSessionChange(() => {
            setHasSession(!!getSession());
        }),
    []);

    const handleLoggedIn = useCallback(() => setHasSession(true), []);
    const handleLoggedOut = useCallback(() => setHasSession(false), []);

    let body: React.ReactNode;
    if (!ready) {
        body = (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin size="large" />
            </div>
        );
    } else if (!hasSession) {
        body = <Login onLoggedIn={handleLoggedIn} />;
    } else {
        body = <Workspace onLoggedOut={handleLoggedOut} />;
    }

    return <ConfigProvider theme={antTheme}>{body}</ConfigProvider>;
}
