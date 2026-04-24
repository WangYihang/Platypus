import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";
import { Outlet, useParams } from "react-router-dom";
import { Loader2 } from "lucide-react";

import EmptyState from "../components/EmptyState";
import StatusBar from "../components/StatusBar";
import { Project, listProjects } from "../lib/api";
import { getSessionUser, getSession } from "../lib/auth";
import { listServers, setActiveServerId } from "../lib/servers";
import {
    GlobalTerminalProvider,
    useGlobalTerminal,
} from "../terminal/GlobalTerminalContext";
import TerminalDrawer from "../terminal/TerminalDrawer";
import CommandPalette from "./CommandPalette";
import AddServerDialog from "./AddServerDialog";
import ManageServersDialog from "./ManageServersDialog";
import ServerRail from "./ServerRail";
import { palette } from "./theme";
import ProjectSidebar from "./ProjectSidebar";

interface ShellState {
    projects: Project[];
    project: Project | null;
    refresh: () => Promise<void>;
}

const ProjectShellContext = createContext<ShellState | null>(null);

export function useShell(): ShellState {
    const ctx = useContext(ProjectShellContext);
    if (!ctx) throw new Error("useShell outside ProjectShell");
    return ctx;
}

export function useCurrentProject(): Project {
    const { project } = useShell();
    if (!project) throw new Error("useCurrentProject called outside a project route");
    return project;
}

interface Props {
    // requireProject is set on routes nested under /projects/:projectSlug; if
    // the slug doesn't resolve we render a not-found state instead of the
    // outlet so child pages don't have to defend.
    requireProject?: boolean;
}

// ProjectShell is the route outlet for every authenticated screen. Owns
// the project list (single fetch shared across pages), resolves
// :projectSlug → Project for nested routes, and renders the sidebar +
// outlet two-column layout.
export default function ProjectShell({ requireProject = false }: Props) {
    const params = useParams<{ projectSlug?: string }>();
    const user = getSessionUser();
    const serverURL = getSession()?.serverURL ?? "";
    const [projects, setProjects] = useState<Project[] | null>(null);

    const refresh = useCallback(async () => {
        try {
            setProjects(await listProjects());
        } catch {
            setProjects([]);
        }
    }, []);

    useEffect(() => {
        void refresh();
    }, [refresh]);

    if (!user) return null;

    const projectList = projects ?? [];
    const project =
        projects && params.projectSlug
            ? projects.find((p) => p.slug === params.projectSlug) ?? null
            : null;
    const loading = projects === null;

    return (
        <ProjectShellContext.Provider
            value={{ projects: projectList, project, refresh }}
        >
            <GlobalTerminalProvider>
                <ShellChrome
                    user={user}
                    serverURL={serverURL}
                    projects={projectList}
                    currentSlug={params.projectSlug}
                    refresh={refresh}
                >
                    {loading ? (
                        <Centered>
                            <Loader2 className="size-5 animate-spin text-text-muted" />
                        </Centered>
                    ) : requireProject && !project ? (
                        <EmptyState
                            title="Project not found"
                            description={`No project with slug "${params.projectSlug}". It may have been deleted, or you may have lost access.`}
                            fill
                        />
                    ) : (
                        <Outlet />
                    )}
                </ShellChrome>
            </GlobalTerminalProvider>
        </ProjectShellContext.Provider>
    );
}

function ShellChrome({
    user,
    serverURL,
    projects,
    currentSlug,
    refresh,
    children,
}: {
    user: ReturnType<typeof getSessionUser> & {};
    serverURL: string;
    projects: Project[];
    currentSlug?: string;
    refresh: () => Promise<void>;
    children: ReactNode;
}) {
    const [addOpen, setAddOpen] = useState(false);
    const [manageOpen, setManageOpen] = useState(false);
    useGlobalTerminalHotkey();
    useServerSwitchHotkeys();
    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                height: "100vh",
                background: palette.main,
                color: palette.textPrimary,
                overflow: "hidden",
            }}
        >
            <div
                style={{
                    display: "flex",
                    flex: 1,
                    minHeight: 0,
                    overflow: "hidden",
                }}
            >
                <ServerRail
                    onAddServer={() => setAddOpen(true)}
                    onManageServers={() => setManageOpen(true)}
                />
                <ProjectSidebar
                    user={user}
                    serverURL={serverURL}
                    projects={projects}
                    currentSlug={currentSlug}
                    onProjectsChanged={() => void refresh()}
                />
                <main style={{ flex: 1, minWidth: 0, overflow: "auto" }}>{children}</main>
            </div>
            <TerminalDrawer />
            <StatusBar />
            <CommandPalette
                onAddServer={() => setAddOpen(true)}
                onManageServers={() => setManageOpen(true)}
            />
            <AddServerDialog open={addOpen} onOpenChange={setAddOpen} />
            <ManageServersDialog
                open={manageOpen}
                onOpenChange={setManageOpen}
                onAddServer={() => setAddOpen(true)}
            />
        </div>
    );
}

// Ctrl+` / Cmd+` toggles the global terminal drawer (VSCode parity).
// Registered at the shell level so the binding is alive on every page.
function useGlobalTerminalHotkey() {
    const { toggleDrawer } = useGlobalTerminal();
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if ((e.ctrlKey || e.metaKey) && e.key === "`") {
                e.preventDefault();
                toggleDrawer();
            }
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [toggleDrawer]);
}

// Ctrl+1..9 (Cmd+1..9 on Mac desktop builds) jumps to the Nth server
// in the rail. On web Chrome reserves Cmd+1..9 for tab switching —
// Alt+1..9 is the web-mode fallback. Both bindings coexist; the
// keydown handler only fires when the combo is free.
function useServerSwitchHotkeys() {
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key < "1" || e.key > "9") return;
            const isMac = /Mac/i.test(navigator.platform);
            const primary = (isMac && e.metaKey) || (!isMac && e.ctrlKey);
            const secondary = e.altKey && !e.ctrlKey && !e.metaKey;
            if (!primary && !secondary) return;
            const idx = Number(e.key) - 1;
            const servers = listServers();
            if (idx < 0 || idx >= servers.length) return;
            e.preventDefault();
            setActiveServerId(servers[idx].id);
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);
}

function Centered({ children }: { children: ReactNode }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                height: "100%",
            }}
        >
            {children}
        </div>
    );
}
