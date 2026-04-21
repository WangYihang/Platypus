import { useEffect, useMemo, useState } from "react";
import { Spin } from "antd";
import { AppstoreOutlined, FolderOpenOutlined } from "@ant-design/icons";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import MainHeader from "../layout/MainHeader";
import AppShell from "../layout/AppShell";
import ProfileRail from "../layout/ProfileRail";
import Sidebar, { Selection } from "../layout/Sidebar";
import { palette, space } from "../layout/theme";
import { Project, listProjects } from "../lib/api";
import { fromNow } from "../lib/time";
import { getSession, getSessionUser, onSessionChange } from "../lib/auth";
import AdminUsers from "./admin/AdminUsers";
import DispatchPanel from "./DispatchPanel";
import HostView from "./HostView";
import ListenerView from "./ListenerView";
import ProjectMembers from "./ProjectMembers";
import ProjectOverview from "./ProjectOverview";

interface Props {
    onLoggedOut: () => void;
}

// Workspace is the post-login root. It composes ProfileRail + Sidebar
// and renders the appropriate main-panel view for the current Selection.
// When nothing is selected it shows a "Projects" landing grid so the
// main panel is never a lonely void.
export default function Workspace({ onLoggedOut }: Props) {
    const [user, setUser] = useState(getSessionUser());
    const [serverURL, setServerURL] = useState(getSession()?.serverURL ?? "");
    const [selection, setSelection] = useState<Selection | null>(null);
    const [projectsByID, setProjectsByID] = useState<Record<string, Project> | null>(null);

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
                setProjectsByID({});
            }
        })();
    }, []);

    if (!user || !serverURL) return null;

    return (
        <AppShell
            profileRail={
                <ProfileRail
                    user={user}
                    serverURL={serverURL}
                    onLoggedOut={onLoggedOut}
                    onOpenAdmin={
                        user.role === "admin"
                            ? () => setSelection({ kind: "admin-users" })
                            : undefined
                    }
                />
            }
            sidebar={<Sidebar selection={selection} onSelect={setSelection} />}
            main={
                <MainPanel
                    selection={selection}
                    projects={projectsByID}
                    onSelect={setSelection}
                />
            }
        />
    );
}

function MainPanel({
    selection,
    projects,
    onSelect,
}: {
    selection: Selection | null;
    projects: Record<string, Project> | null;
    onSelect: (s: Selection) => void;
}) {
    if (!selection) {
        return <ProjectsLanding projects={projects} onSelect={onSelect} />;
    }
    if (selection.kind === "admin-users") {
        return <AdminUsers />;
    }
    if (!projects) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin />
            </div>
        );
    }
    const project = projects[selection.projectId];
    if (!project) {
        return (
            <EmptyState
                title="Project not found"
                description="It may have been deleted, or you may have lost access. Pick something else in the sidebar."
                fill
            />
        );
    }

    switch (selection.kind) {
        case "overview":
            return (
                <ProjectOverview
                    project={project}
                    onOpenMembers={() =>
                        onSelect({ kind: "project-members", projectId: project.id })
                    }
                />
            );
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
        case "project-members":
            return <ProjectMembers project={project} />;
    }
}

// ProjectsLanding is the default main-panel view when no sidebar entity
// is selected. It tiles the projects so the user lands on *something*
// actionable instead of a floating sentence.
function ProjectsLanding({
    projects,
    onSelect,
}: {
    projects: Record<string, Project> | null;
    onSelect: (s: Selection) => void;
}) {
    const list = useMemo(() => {
        if (!projects) return null;
        return Object.values(projects).sort((a, b) => a.name.localeCompare(b.name));
    }, [projects]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <MainHeader
                title="Projects"
                subtitle="Pick a project to inspect its hosts, listeners, and dispatch"
            />
            <div
                style={{
                    flex: 1,
                    overflow: "auto",
                    padding: space[6],
                }}
            >
                <div style={{ maxWidth: 1200, margin: "0 auto" }}>
                    {!list && (
                        <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                            <Spin />
                        </div>
                    )}
                    {list && list.length === 0 && (
                        <EmptyState
                            icon={<FolderOpenOutlined />}
                            title="No projects yet"
                            description="An admin creates projects from the sidebar. Each project groups its own listeners, hosts, sessions, and dispatches."
                        />
                    )}
                    {list && list.length > 0 && (
                        <div
                            style={{
                                display: "grid",
                                gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
                                gap: space[3],
                            }}
                        >
                            {list.map((p) => (
                                <ProjectTile
                                    key={p.id}
                                    project={p}
                                    onOpen={() =>
                                        onSelect({ kind: "overview", projectId: p.id })
                                    }
                                />
                            ))}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

function ProjectTile({ project, onOpen }: { project: Project; onOpen: () => void }) {
    return (
        <Card interactive onClick={onOpen} padding={5}>
            <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
                <div
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: space[2],
                        color: palette.textMuted,
                        fontSize: 12,
                    }}
                >
                    <AppstoreOutlined />
                    <Mono size={11}>{project.slug}</Mono>
                </div>
                <div
                    style={{
                        color: palette.textPrimary,
                        fontSize: 18,
                        fontWeight: 600,
                        lineHeight: 1.2,
                    }}
                >
                    {project.name}
                </div>
                <div
                    style={{
                        color: palette.textSecondary,
                        fontSize: 12,
                    }}
                >
                    created {fromNow(project.created_at)}
                </div>
            </div>
        </Card>
    );
}
