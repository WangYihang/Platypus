import { useMemo, useState } from "react";
import { ChevronDown, Search, SlashIcon } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Project } from "../lib/api";
import { SessionUser } from "../lib/auth";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "@/components/ui/popover";

import { ProjectSwitcherMenu } from "./ProjectSwitcher";
import ServerSwitcher from "./ServerSwitcher";
import UserMenu from "./UserMenu";
import { palette, radius, space } from "./theme";

interface Props {
    user: SessionUser;
    serverURL: string;
    projects: Project[];
    currentProject?: Project | null;
    onCreateProject?: () => void;
    onAddServer: () => void;
    onManageServers: () => void;
}

// TopBar is the single line of chrome at the top of every authenticated
// screen. Replaces the previous left-rail sidebar entirely:
//
//   ◇ Platypus / <project> ▾ / <server> ▾    [ ⌘K ]    ◐ user ▾
//
// `<project>` only renders inside a project context (currentProject != null);
// in global routes (`/projects`, `/servers`, `/admin/*`, `/account`,
// `/preferences`) only the server breadcrumb appears.
//
// The horizontal nav row (Overview / Fleet / … or Projects / Servers /
// Admin) is owned by NavTabs.tsx and rendered immediately below this
// bar — they're separate components on purpose so each can re-render
// independently when their inputs change.
export default function TopBar({
    user,
    serverURL,
    projects,
    currentProject,
    onCreateProject,
    onAddServer,
    onManageServers,
}: Props) {
    return (
        <div
            data-testid="top-bar"
            style={{
                flexShrink: 0,
                height: 48,
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: `0 ${space[3]}px`,
                background: palette.rail,
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <Brand />
            <BreadcrumbSep />
            <ServerBreadcrumb
                onAddServer={onAddServer}
                onManageServers={onManageServers}
            />
            {currentProject ? (
                <>
                    <BreadcrumbSep />
                    <ProjectBreadcrumb
                        projects={projects}
                        currentProject={currentProject}
                        canCreateProject={user.role === "admin"}
                        onCreateProject={onCreateProject}
                    />
                </>
            ) : null}

            <div style={{ flex: 1, minWidth: 0 }} />

            <CmdKTrigger />
            <UserMenu user={user} serverURL={serverURL} variant="compact" />
        </div>
    );
}

function Brand() {
    const navigate = useNavigate();
    return (
        <button
            type="button"
            onClick={() => navigate("/projects")}
            data-testid="brand"
            aria-label="Platypus home"
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                padding: "4px 6px",
                background: "transparent",
                border: "none",
                color: palette.textPrimary,
                cursor: "pointer",
                fontSize: 13,
                fontWeight: 600,
                fontFamily: "var(--font-geist-mono)",
            }}
        >
            <span aria-hidden style={{ fontSize: 14 }}>◇</span>
            <span>Platypus</span>
        </button>
    );
}

function BreadcrumbSep() {
    return (
        <SlashIcon
            aria-hidden
            className="size-3.5"
            style={{ color: palette.textMuted, flexShrink: 0 }}
        />
    );
}

interface ProjectBreadcrumbProps {
    projects: Project[];
    currentProject: Project;
    canCreateProject: boolean;
    onCreateProject?: () => void;
}

function ProjectBreadcrumb({
    projects,
    currentProject,
    canCreateProject,
    onCreateProject,
}: ProjectBreadcrumbProps) {
    const [open, setOpen] = useState(false);
    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    data-testid="project-switcher-trigger"
                    className="pl-breadcrumb-pill"
                >
                    <span style={{ minWidth: 0, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                        {currentProject.name}
                    </span>
                    <ChevronDown className="size-3 text-text-muted" />
                </button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-[260px] p-1">
                <ProjectSwitcherMenu
                    projects={projects}
                    currentSlug={currentProject.slug}
                    canCreateProject={canCreateProject}
                    onCreateProject={onCreateProject}
                    onClose={() => setOpen(false)}
                />
            </PopoverContent>
        </Popover>
    );
}

interface ServerBreadcrumbProps {
    onAddServer: () => void;
    onManageServers: () => void;
}

function ServerBreadcrumb({ onAddServer, onManageServers }: ServerBreadcrumbProps) {
    return (
        <ServerSwitcher
            variant="breadcrumb"
            onAddServer={onAddServer}
            onManageServers={onManageServers}
        />
    );
}

function CmdKTrigger() {
    const meta =
        typeof navigator !== "undefined" && /Mac/i.test(navigator.platform || "")
            ? "⌘"
            : "Ctrl";
    function trigger() {
        const evt = new KeyboardEvent("keydown", {
            key: "k",
            ctrlKey: true,
            metaKey: true,
            bubbles: true,
            cancelable: true,
        });
        window.dispatchEvent(evt);
    }
    return (
        <button
            type="button"
            onClick={trigger}
            data-testid="top-bar-cmdk"
            aria-label="Open command palette"
            title={`Open command palette (${meta}+K)`}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: space[2],
                width: 320,
                maxWidth: "30vw",
                height: 28,
                padding: `0 ${space[2]}px 0 10px`,
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                color: palette.textMuted,
                fontFamily: "var(--font-geist-mono)",
                fontSize: 12,
                cursor: "pointer",
                textAlign: "left",
                flexShrink: 0,
            }}
        >
            <Search className="size-3.5" style={{ color: palette.textMuted, flexShrink: 0 }} />
            <span style={{ flex: 1, minWidth: 0 }}>Search or run command…</span>
            <span
                aria-hidden
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 2,
                    padding: "1px 6px",
                    background: palette.main,
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.sm,
                    color: palette.textMuted,
                    fontSize: 10,
                    lineHeight: 1.4,
                    flexShrink: 0,
                }}
            >
                {meta}K
            </span>
        </button>
    );
}
