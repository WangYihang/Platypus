import { useEffect, useState } from "react";
import { Typography } from "antd";

import AppShell from "../layout/AppShell";
import ProfileRail from "../layout/ProfileRail";
import Sidebar, { Selection } from "../layout/Sidebar";
import { palette } from "../layout/theme";
import { Project, listProjects } from "../lib/api";
import { getSession, getSessionUser, onSessionChange } from "../lib/auth";
import DispatchPanel from "./DispatchPanel";
import HostView from "./HostView";
import ListenerView from "./ListenerView";
import ProjectOverview from "./ProjectOverview";

interface Props {
    onLoggedOut: () => void;
}

// Workspace is the post-login root. It composes ProfileRail + Sidebar
// and renders the appropriate main-panel view for the current
// Selection. Project data is cached at this level so each view can
// assume its `project` prop is resolved (no flicker when switching
// between host/listener/overview within the same project).
export default function Workspace({ onLoggedOut }: Props) {
    const [user, setUser] = useState(getSessionUser());
    const [serverURL, setServerURL] = useState(getSession()?.serverURL ?? "");
    const [selection, setSelection] = useState<Selection | null>(null);
    const [projectsByID, setProjectsByID] = useState<Record<string, Project>>({});

    useEffect(() =>
        onSessionChange(() => {
            const s = getSession();
            setUser(s?.user ?? null);
            setServerURL(s?.serverURL ?? "");
        }),
    []);

    useEffect(() => {
        (async () => {
            try {
                const list = await listProjects();
                const map: Record<string, Project> = {};
                for (const p of list) map[p.id] = p;
                setProjectsByID(map);
            } catch {
                // The sidebar also fetches projects — errors surface there.
            }
        })();
    }, []);

    if (!user || !serverURL) return null;

    return (
        <AppShell
            profileRail={
                <ProfileRail user={user} serverURL={serverURL} onLoggedOut={onLoggedOut} />
            }
            sidebar={<Sidebar selection={selection} onSelect={setSelection} />}
            main={<MainPanel selection={selection} projects={projectsByID} onSelect={setSelection} />}
        />
    );
}

function MainPanel({
    selection,
    projects,
    onSelect,
}: {
    selection: Selection | null;
    projects: Record<string, Project>;
    onSelect: (s: Selection) => void;
}) {
    if (!selection) {
        return <EmptyState />;
    }
    const project = projects[selection.projectId];
    if (!project) {
        // Project still loading or was deleted — show a minimal stub.
        return <EmptyState message="Loading project…" />;
    }

    switch (selection.kind) {
        case "overview":
            return <ProjectOverview project={project} />;
        case "host":
            return <HostView projectID={project.id} hostID={selection.hostId} />;
        case "listener":
            return (
                <ListenerView
                    projectID={project.id}
                    listenerID={selection.listenerId}
                    onSelectListener={(lid) =>
                        onSelect({
                            kind: "listener",
                            projectId: project.id,
                            listenerId: lid,
                        })
                    }
                />
            );
        case "dispatch":
            return <DispatchPanel projectID={project.id} projectName={project.name} />;
    }
}

function EmptyState({ message }: { message?: string }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                height: "100%",
            }}
        >
            <Typography.Text style={{ color: palette.textSecondary }}>
                {message || "Pick a project, host, or listener in the sidebar."}
            </Typography.Text>
        </div>
    );
}
