import { ReactNode, useState } from "react";
import { Form, Input, Modal, message } from "antd";
import { NavLink } from "react-router-dom";
import {
    AppstoreOutlined,
    CloudDownloadOutlined,
    DesktopOutlined,
    GatewayOutlined,
    SafetyOutlined,
    TeamOutlined,
    ThunderboltOutlined,
} from "@ant-design/icons";

import Brand from "../components/Brand";
import { Project, createProject } from "../lib/api";
import { SessionUser } from "../lib/auth";
import { palette, space } from "./theme";
import ProjectSwitcher from "./ProjectSwitcher";
import UserMenu from "./UserMenu";

interface Props {
    user: SessionUser;
    serverURL: string;
    projects: Project[];
    currentSlug?: string;
    onProjectsChanged: () => void;
}

interface NavItem {
    to: string;
    label: string;
    icon: ReactNode;
    requiresProject: boolean;
    minRole?: SessionUser["role"];
}

// ProjectSidebar is the left rail. Linear/Resend-style: brand at top,
// project switcher dropdown, flat per-project nav, user menu pinned at
// the bottom. Replaces the old AppShell + ProfileRail + Sidebar tree.
export default function ProjectSidebar({
    user,
    serverURL,
    projects,
    currentSlug,
    onProjectsChanged,
}: Props) {
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm] = Form.useForm<{ name: string; slug: string }>();
    const [messageApi, contextHolder] = message.useMessage();

    const items: NavItem[] = [
        { to: "overview", label: "Overview", icon: <AppstoreOutlined />, requiresProject: true },
        { to: "hosts", label: "Hosts", icon: <DesktopOutlined />, requiresProject: true },
        { to: "listeners", label: "Listeners", icon: <GatewayOutlined />, requiresProject: true },
        { to: "sessions", label: "Sessions", icon: <SafetyOutlined />, requiresProject: true },
        { to: "enrollment", label: "Enrollment", icon: <CloudDownloadOutlined />, requiresProject: true, minRole: "admin" },
        { to: "dispatch", label: "Dispatch", icon: <ThunderboltOutlined />, requiresProject: true, minRole: "operator" },
        { to: "members", label: "Members", icon: <TeamOutlined />, requiresProject: true, minRole: "operator" },
    ];

    const visible = items.filter((it) => meetsRole(user.role, it.minRole));

    async function handleCreateProject() {
        const v = await createForm.validateFields();
        try {
            await createProject(v.name, v.slug);
            messageApi.success(`Created project ${v.slug}`);
            setCreateOpen(false);
            createForm.resetFields();
            onProjectsChanged();
        } catch (e) {
            messageApi.error(`create: ${String(e)}`);
        }
    }

    return (
        <aside
            style={{
                width: 240,
                height: "100%",
                background: palette.sidebar,
                borderRight: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
                flexShrink: 0,
            }}
        >
            {contextHolder}

            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[4]}px ${space[3]}px ${space[3]}px`,
                }}
            >
                <Brand />
                <span
                    style={{
                        fontWeight: 600,
                        color: palette.textPrimary,
                        fontSize: 14,
                        letterSpacing: -0.2,
                    }}
                >
                    Platypus
                </span>
            </div>

            <div style={{ padding: `0 ${space[3]}px ${space[3]}px` }}>
                <ProjectSwitcher
                    projects={projects}
                    currentSlug={currentSlug}
                    canCreateProject={user.role === "admin"}
                    onCreateProject={() => setCreateOpen(true)}
                />
            </div>

            <nav style={{ flex: 1, padding: `${space[2]}px ${space[2]}px`, overflow: "auto" }}>
                {currentSlug ? (
                    visible.map((it) => (
                        <NavLink
                            key={it.to}
                            to={`/projects/${currentSlug}/${it.to}`}
                            className={({ isActive }) =>
                                "pl-nav-link" + (isActive ? " pl-nav-link--active" : "")
                            }
                        >
                            <span style={{ width: 16, display: "inline-flex", justifyContent: "center" }}>
                                {it.icon}
                            </span>
                            <span>{it.label}</span>
                        </NavLink>
                    ))
                ) : (
                    <div
                        style={{
                            padding: `${space[3]}px ${space[3]}px`,
                            color: palette.textMuted,
                            fontSize: 12,
                            lineHeight: 1.5,
                        }}
                    >
                        Pick a project to see its hosts, listeners, and sessions.
                    </div>
                )}
            </nav>

            <UserMenu user={user} serverURL={serverURL} />

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
                        extra="URL-safe id, unique across projects."
                    >
                        <Input placeholder="prod" />
                    </Form.Item>
                </Form>
            </Modal>
        </aside>
    );
}

function meetsRole(actual: SessionUser["role"], required?: SessionUser["role"]): boolean {
    if (!required) return true;
    const order = { viewer: 0, operator: 1, admin: 2 };
    return order[actual] >= order[required];
}
