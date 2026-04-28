import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";
import { Outlet, useParams } from "react-router-dom";
import { Loader2 } from "lucide-react";

import EmptyState from "../components/EmptyState";
import StatusBar from "../components/StatusBar";
import { TransfersDrawer, TransfersDrawerProvider } from "../components/TransfersPill";
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
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "@/components/ui/resizable";

interface ShellState {
    projects: Project[];
    project: Project | null;
    refresh: () => Promise<void>;
    // loading is true while the initial /projects fetch is in flight.
    // Surfaced so routes that render at a loading state (today:
    // ProjectsLanding) can show their own skeleton instead of the
    // shell-level Loader2 spinner.
    loading: boolean;
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
            value={{ projects: projectList, project, refresh, loading }}
        >
            <GlobalTerminalProvider>
                <TransfersDrawerProvider>
                    <ShellChrome
                        user={user}
                        serverURL={serverURL}
                        projects={projectList}
                        currentSlug={params.projectSlug}
                        refresh={refresh}
                    >
                    {loading && requireProject ? (
                        // For project-scoped routes (Fleet, Members,
                        // Settings, …) the slug must resolve to a real
                        // project before the page can render anything
                        // useful, so we still hold them with the
                        // shell-level spinner. Non-project routes
                        // (today: ProjectsLanding at /projects) get
                        // the Outlet during loading and render their
                        // own skeleton via useShell().loading.
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
                </TransfersDrawerProvider>
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
                {/* ResizablePanelGroup owns the sidebar ↔ main split.
                    ServerRail stays outside the group because it's a
                    narrow icon column with no interesting resize range
                    (RAIL_WIDTH constant). The sidebar starts at 240px
                    to match the previous fixed width; persisted layout
                    is keyed under platypus.layout.shell-sidebar so a
                    user's choice survives reloads. */}
                <ResizablePanelGroup
                    direction="horizontal"
                    autoSaveId="shell-sidebar"
                    style={{ flex: 1, minHeight: 0, minWidth: 0 }}
                >
                    <ResizablePanel
                        id="sidebar"
                        defaultSize="240px"
                        minSize="180px"
                        maxSize="480px"
                        className="flex"
                    >
                        <ProjectSidebar
                            user={user}
                            serverURL={serverURL}
                            projects={projects}
                            currentSlug={currentSlug}
                            onProjectsChanged={() => void refresh()}
                        />
                    </ResizablePanel>
                    <ResizableHandle />
                    <ResizablePanel id="main" minSize="40%" className="flex">
                        <div
                            style={{
                                flex: 1,
                                minWidth: 0,
                                minHeight: 0,
                                display: "flex",
                                flexDirection: "column",
                                // position: relative anchors the absolutely
                                // positioned TransfersDrawer to the main pane
                                // so it slides in from the right edge of the
                                // outlet (not the viewport).
                                position: "relative",
                            }}
                        >
                    <main style={{ flex: 1, minWidth: 0, minHeight: 0, overflow: "auto" }}>
                        {/* P3: cap the rendered content width on wide
                            displays. Without the cap, KPI cards, tables
                            and member lists stretched 1500+ px wide on
                            1920px monitors — long lines are readable
                            but tiring to scan. 1280px keeps text rows
                            in roughly the 80-100 character sweet spot
                            without leaving big black voids on either
                            side at narrower widths (the cap is a
                            max, not a fixed value). */}
                        <div
                            data-testid="shell-content-frame"
                            style={{
                                maxWidth: 1280,
                                margin: "0 auto",
                                // height (not minHeight) so child pages
                                // that rely on `height: 100%` for
                                // absolute-positioned regions (e.g.
                                // FleetPage's three swap-via-display
                                // panels) keep a defined parent
                                // height to compute against.
                                height: "100%",
                                display: "flex",
                                flexDirection: "column",
                            }}
                        >
                            {children}
                        </div>
                    </main>
                    <TerminalDrawer />
                    <TransfersDrawer />
                        </div>
                    </ResizablePanel>
                </ResizablePanelGroup>
            </div>
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
