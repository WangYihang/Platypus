import { useEffect, useState } from "react";
import { Typography } from "antd";

import AppShell from "../layout/AppShell";
import ProfileRail from "../layout/ProfileRail";
import Sidebar, { Selection } from "../layout/Sidebar";
import { palette } from "../layout/theme";
import { getSession, getSessionUser, onSessionChange } from "../lib/auth";

interface Props {
    onLoggedOut: () => void;
}

// Workspace is the post-login root of the new Slack-style UI. It wires
// ProfileRail + Sidebar into the AppShell and drives the selection
// state that P10's HostView / ListenerView / DispatchPanel will
// consume. For this commit the main region shows a placeholder so the
// sidebar flow is end-to-end testable (log in → see projects → click
// a host → see the selection echoed).
export default function Workspace({ onLoggedOut }: Props) {
    const [user, setUser] = useState(getSessionUser());
    const [serverURL, setServerURL] = useState(getSession()?.serverURL ?? "");
    const [selection, setSelection] = useState<Selection | null>(null);

    useEffect(() =>
        onSessionChange(() => {
            const s = getSession();
            setUser(s?.user ?? null);
            setServerURL(s?.serverURL ?? "");
        }),
    []);

    if (!user || !serverURL) {
        // Session was cleared by the auth layer — kick up to WebShell.
        return null;
    }

    return (
        <AppShell
            profileRail={
                <ProfileRail user={user} serverURL={serverURL} onLoggedOut={onLoggedOut} />
            }
            sidebar={<Sidebar selection={selection} onSelect={setSelection} />}
            main={<MainPlaceholder selection={selection} />}
        />
    );
}

function MainPlaceholder({ selection }: { selection: Selection | null }) {
    return (
        <div style={{ padding: 24 }}>
            <Typography.Title level={4} style={{ color: palette.textPrimary, marginTop: 0 }}>
                {selection ? describe(selection) : "Select a host, listener, or project."}
            </Typography.Title>
            <pre
                style={{
                    color: palette.textSecondary,
                    background: palette.sidebar,
                    padding: 12,
                    borderRadius: 6,
                    border: `1px solid ${palette.border}`,
                    whiteSpace: "pre-wrap",
                }}
            >
                {selection ? JSON.stringify(selection, null, 2) : "// nothing selected"}
            </pre>
            <Typography.Paragraph style={{ color: palette.textSecondary, marginTop: 16 }}>
                Main-panel views (Terminal / Files / Tunnels / Sessions / Info) land in P10.
            </Typography.Paragraph>
        </div>
    );
}

function describe(s: Selection): string {
    switch (s.kind) {
        case "overview":
            return "Project overview";
        case "host":
            return "Host selected";
        case "listener":
            return "Listener selected";
        case "dispatch":
            return "Dispatch";
    }
}
