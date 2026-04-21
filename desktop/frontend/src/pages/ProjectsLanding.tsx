import { useMemo } from "react";
import { Spin } from "antd";
import { AppstoreOutlined, FolderOpenOutlined } from "@ant-design/icons";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import MainHeader from "../layout/MainHeader";
import { useShell } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { Project } from "../lib/api";
import { fromNow } from "../lib/time";

// ProjectsLanding is the / and /projects route. Tile grid of every
// project the user can see; clicking a tile navigates into that
// project's overview.
export default function ProjectsLanding() {
    const { projects } = useShell();
    const navigate = useNavigate();

    const list = useMemo(
        () => [...projects].sort((a, b) => a.name.localeCompare(b.name)),
        [projects],
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <MainHeader
                title="Projects"
                subtitle="Pick a project to inspect its hosts, listeners, and dispatch"
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {list.length === 0 ? (
                    <EmptyState
                        icon={<FolderOpenOutlined />}
                        title="No projects yet"
                        description="An admin creates projects from the sidebar. Each project groups its own listeners, hosts, sessions, and dispatches."
                    />
                ) : (
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
                                onOpen={() => navigate(`/projects/${p.slug}/overview`)}
                            />
                        ))}
                    </div>
                )}
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
                <div style={{ color: palette.textSecondary, fontSize: 12 }}>
                    created {fromNow(project.created_at)}
                </div>
            </div>
        </Card>
    );
}
