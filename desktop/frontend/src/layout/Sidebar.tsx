import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button, Form, Input, Modal, Spin, message } from "antd";
import { PlusOutlined, SearchOutlined } from "@ant-design/icons";

import {
    Host,
    Listener,
    Project,
    createProject,
    listHosts,
    listListeners,
    listProjects,
} from "../lib/api";
import { getSessionUser } from "../lib/auth";
import {
    HostSeenPayload,
    ListenerEventPayload,
    NotifyEvent,
    onNotify,
} from "../lib/notify";
import { palette } from "./theme";
import ProjectSection from "./ProjectSection";

// Selection describes what the main panel should render. "overview"
// shows a project's counts; "admin-users" is the global-admin settings
// surface accessed from the profile rail; everything else zooms into
// a project-scoped entity.
export type Selection =
    | { kind: "overview"; projectId: string }
    | { kind: "listener"; projectId: string; listenerId: string }
    | { kind: "host"; projectId: string; hostId: string }
    | { kind: "dispatch"; projectId: string }
    | { kind: "project-members"; projectId: string }
    | { kind: "admin-users" };

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
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm] = Form.useForm<{ name: string; slug: string }>();
    const [messageApi, contextHolder] = message.useMessage();
    const searchRef = useRef<HTMLInputElement | null>(null);

    const user = getSessionUser();
    const canCreateProject = user?.role === "admin";

    const loadProjects = useCallback(async () => {
        try {
            const list = await listProjects();
            const data = await Promise.all(
                list.map(async (p) => ({
                    project: p,
                    listeners: await listListeners(p.id),
                    hosts: await listHosts(p.id),
                })),
            );
            setProjects(data);
            setError(null);
        } catch (err) {
            setError(String(err));
        }
    }, []);

    // reloadProject refetches the listener + host lists for a single
    // project and replaces that project's entry in-place. Used by the
    // event subscribers so incoming host.seen / listener.* events
    // don't trigger a full sidebar reload.
    const reloadProject = useCallback(async (projectID: string) => {
        try {
            const [ls, hs] = await Promise.all([
                listListeners(projectID),
                listHosts(projectID),
            ]);
            setProjects((prev) =>
                prev
                    ? prev.map((p) =>
                          p.project.id === projectID
                              ? { project: p.project, listeners: ls, hosts: hs }
                              : p,
                      )
                    : prev,
            );
        } catch {
            // ignore — next full reload will catch up
        }
    }, []);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            if (!cancelled) await loadProjects();
        })();
        return () => {
            cancelled = true;
        };
    }, [loadProjects]);

    // Subscribe to lifecycle events so host / listener changes appear
    // without a manual refresh. Each handler calls reloadProject for
    // the affected project rather than re-running the full project list
    // — keeps the invalidation scope minimal.
    useEffect(() => {
        const offs: Array<() => void> = [];
        offs.push(
            onNotify(NotifyEvent.HostSeen, (data) => {
                const p = data as HostSeenPayload;
                if (p?.project_id) void reloadProject(p.project_id);
            }),
        );
        for (const evt of [NotifyEvent.ListenerCreated, NotifyEvent.ListenerDeleted]) {
            offs.push(
                onNotify(evt, (data) => {
                    const p = data as ListenerEventPayload;
                    if (p?.project_id) void reloadProject(p.project_id);
                }),
            );
        }
        return () => offs.forEach((off) => off());
    }, [reloadProject]);

    // Cmd/Ctrl+K focuses the search input so operators running a large
    // fleet can jump to a host without reaching for the mouse.
    useEffect(() => {
        function onKey(e: KeyboardEvent) {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
                e.preventDefault();
                searchRef.current?.focus();
            }
        }
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);

    async function handleCreateProject() {
        const v = await createForm.validateFields();
        try {
            await createProject(v.name, v.slug);
            messageApi.success(`Created project ${v.slug}`);
            setCreateOpen(false);
            createForm.resetFields();
            await loadProjects();
        } catch (e) {
            messageApi.error(`create: ${String(e)}`);
        }
    }

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
            {contextHolder}
            <div
                style={{
                    padding: "12px 12px 8px",
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <Input
                    ref={(el: { input: HTMLInputElement | null } | null) => {
                        searchRef.current = el?.input ?? null;
                    }}
                    placeholder="Search hosts, listeners…  (⌘K)"
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
                {filtered?.length === 0 && !canCreateProject && (
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
                {canCreateProject && (
                    <div style={{ padding: "12px 12px 16px" }}>
                        <Button
                            block
                            icon={<PlusOutlined />}
                            onClick={() => setCreateOpen(true)}
                            size="small"
                        >
                            New project
                        </Button>
                    </div>
                )}
            </div>

            <Modal
                title="New project"
                open={createOpen}
                onOk={handleCreateProject}
                onCancel={() => setCreateOpen(false)}
                okText="Create"
                destroyOnHidden
            >
                <Form form={createForm} layout="vertical">
                    <Form.Item
                        name="name"
                        label="Project name"
                        rules={[{ required: true }]}
                        extra="Human-friendly — shown in the sidebar header."
                    >
                        <Input autoFocus placeholder="Production" />
                    </Form.Item>
                    <Form.Item
                        name="slug"
                        label="Slug"
                        rules={[
                            { required: true },
                            {
                                pattern: /^[a-z0-9][a-z0-9_-]{0,62}$/,
                                message: "a-z, 0-9, _ and - only; must start alphanumeric",
                            },
                        ]}
                        extra="URL-safe id, unique across projects. Becomes /projects/<slug> in the API."
                    >
                        <Input placeholder="prod" />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
