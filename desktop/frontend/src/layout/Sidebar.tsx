import { useEffect, useMemo, useState } from "react";
import { Input, Spin } from "antd";
import { SearchOutlined } from "@ant-design/icons";

import { Host, Listener, Project, listHosts, listListeners, listProjects } from "../lib/api";
import { palette } from "./theme";
import ProjectSection from "./ProjectSection";

// Selection describes what the main panel should render. "overview"
// shows a project's counts; everything else zooms into an entity.
export type Selection =
    | { kind: "overview"; projectId: string }
    | { kind: "listener"; projectId: string; listenerId: string }
    | { kind: "host"; projectId: string; hostId: string }
    | { kind: "dispatch"; projectId: string };

interface Props {
    selection: Selection | null;
    onSelect: (s: Selection) => void;
}

interface ProjectData {
    project: Project;
    listeners: Listener[];
    hosts: Host[];
}

// Sidebar holds the search input + one collapsible block per project
// the user can see. Data loads once on mount; a per-project refresh is
// triggered when the user selects into one of its entities (P10 adds
// that path). For this commit the focus is structural — the data shape
// and the selection callbacks.
export default function Sidebar({ selection, onSelect }: Props) {
    const [projects, setProjects] = useState<ProjectData[] | null>(null);
    const [query, setQuery] = useState("");
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const list = await listProjects();
                const data = await Promise.all(
                    list.map(async (p) => ({
                        project: p,
                        listeners: await listListeners(p.id),
                        hosts: await listHosts(p.id),
                    })),
                );
                if (!cancelled) setProjects(data);
            } catch (err) {
                if (!cancelled) setError(String(err));
            }
        })();
        return () => {
            cancelled = true;
        };
    }, []);

    // Filter: case-insensitive prefix on hostname/alias/host:port/slug.
    // We filter at the Sidebar level so ProjectSection doesn't have to
    // re-run the match on every keystroke.
    const q = query.trim().toLowerCase();
    const filtered = useMemo<ProjectData[] | null>(() => {
        if (!projects) return null;
        if (!q) return projects;
        return projects.map((p) => ({
            project: p.project,
            listeners: p.listeners.filter((l) =>
                `${l.host}:${l.port}`.toLowerCase().includes(q),
            ),
            hosts: p.hosts.filter((h) =>
                [h.hostname, h.primary_alias, h.os, h.machine_id]
                    .filter(Boolean)
                    .some((v) => String(v).toLowerCase().includes(q)),
            ),
        }));
    }, [projects, q]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <div
                style={{
                    padding: "12px 12px 8px",
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <Input
                    placeholder="Search hosts, listeners…"
                    prefix={<SearchOutlined style={{ color: palette.textSecondary }} />}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    size="middle"
                    allowClear
                />
            </div>

            <div style={{ flex: 1, overflow: "auto", padding: "8px 0" }}>
                {error && (
                    <div style={{ padding: 12, color: palette.danger }}>{error}</div>
                )}
                {!filtered && !error && (
                    <div style={{ display: "flex", justifyContent: "center", padding: 24 }}>
                        <Spin />
                    </div>
                )}
                {filtered?.length === 0 && (
                    <div
                        style={{
                            padding: 16,
                            color: palette.textSecondary,
                            fontSize: 13,
                        }}
                    >
                        No projects yet. An admin must create one.
                    </div>
                )}
                {filtered?.map((p) => (
                    <ProjectSection
                        key={p.project.id}
                        project={p.project}
                        listeners={p.listeners}
                        hosts={p.hosts}
                        selection={selection}
                        onSelect={onSelect}
                    />
                ))}
            </div>
        </div>
    );
}
