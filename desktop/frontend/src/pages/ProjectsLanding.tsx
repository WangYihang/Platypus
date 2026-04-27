import { useMemo } from "react";
import { FolderOpen, LayoutGrid } from "lucide-react";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import { useShell } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { Project } from "../lib/api";
import { getSessionUser } from "../lib/auth";
import { fromNow } from "../lib/time";
import { Skeleton } from "@/components/ui/skeleton";

// ProjectsLanding is the / and /projects route. Tile grid of every
// project the user can see; clicking a tile navigates into that
// project's overview.
export default function ProjectsLanding() {
    const { projects, loading } = useShell();
    const navigate = useNavigate();
    const user = getSessionUser();

    const list = useMemo(
        () => [...projects].sort((a, b) => a.name.localeCompare(b.name)),
        [projects],
    );

    // Empty-state copy varies by role: admins get an action prompt
    // (they can create one), everyone else gets a "talk to your
    // admin" message — operators/viewers don't see the New project
    // button at all, so the original "An admin creates projects
    // from the sidebar" sent them looking for a button that didn't
    // exist for them.
    const emptyDescription =
        user?.role === "admin"
            ? "Use 'New project' in the sidebar to create projects from the sidebar. Each project scopes its own hosts, sessions, and access."
            : "Ask your admin to create a project or invite you to an existing one. Projects scope hosts, sessions, and access — you'll see them here once you have access.";

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Projects"
                subtitle="Pick a project to inspect its fleet and activity"
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {loading ? (
                    <ProjectsLandingSkeleton />
                ) : list.length === 0 ? (
                    <EmptyState
                        icon={<FolderOpen className="size-5" />}
                        title="No projects yet"
                        description={emptyDescription}
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

// ProjectsLandingSkeleton matches the populated tile grid's column
// shape (auto-fill, minmax(260px, 1fr)) so the swap from loading to
// loaded is a content swap, not a layout pop. Six tiles is enough to
// fill a typical viewport without going overboard.
function ProjectsLandingSkeleton() {
    const tiles = Array.from({ length: 6 });
    return (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
                gap: space[3],
            }}
        >
            {tiles.map((_, i) => (
                <Card
                    key={i}
                    padding={5}
                    data-testid="project-tile-skeleton"
                >
                    <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
                        <div
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                            }}
                        >
                            <Skeleton className="size-3.5 rounded" />
                            <Skeleton className="h-3 w-20" />
                        </div>
                        <Skeleton className="h-5 w-40" />
                        <Skeleton className="h-3 w-28" />
                    </div>
                </Card>
            ))}
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
                    <LayoutGrid className="size-3.5" />
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
