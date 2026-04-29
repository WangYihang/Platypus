import {
    ReactNode,
    createContext,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useState,
} from "react";
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
import TerminalDrawer, { TAB_BAR_HEIGHT } from "../terminal/TerminalDrawer";
import CommandPalette from "./CommandPalette";
import AddServerDialog from "./AddServerDialog";
import ManageServersDialog from "./ManageServersDialog";
import { palette } from "./theme";
import ProjectSidebar from "./ProjectSidebar";
import TopChrome from "./TopChrome";
import { usePreference } from "../lib/preferences";
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
    usePanelRef,
} from "@/components/ui/resizable";
// ResizablePanelGroup is still used by MainColumn for the
// vertical content / terminal-drawer split below; only the
// horizontal sidebar / main split was simplified to a fixed-width
// column above.

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
            {/* TopChrome owns the global Cmd-K trigger — its centered
                input-shaped button dispatches the same keydown event
                the existing CommandPalette listens for, so a click
                and the keyboard shortcut go through one open path.
                Left/right slots are reserved for breadcrumbs and
                global indicators in later phases. */}
            <TopChrome />
            <div
                style={{
                    display: "flex",
                    flex: 1,
                    minHeight: 0,
                    overflow: "hidden",
                }}
            >
                {/* Sidebar is now a fixed-width column driven by the
                    `ui.sidebarExpanded` preference. Default is the
                    72-px icon-only rail; flipping the chevron toggle
                    inside ProjectSidebar expands it to 200 px with
                    nav labels. The previous react-resizable-panels
                    drag-to-size affordance was dropped in the R3
                    redesign — the rail has two states, not a
                    continuum, so two-button toggle is clearer than
                    a draggable seam. */}
                <SidebarColumn>
                    <ProjectSidebar
                        user={user}
                        serverURL={serverURL}
                        projects={projects}
                        currentSlug={currentSlug}
                        onProjectsChanged={() => void refresh()}
                        onAddServer={() => setAddOpen(true)}
                        onManageServers={() => setManageOpen(true)}
                    />
                </SidebarColumn>
                <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
                    <MainColumn>{children}</MainColumn>
                </div>
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

// SidebarColumn picks the rail width from the user's
// `ui.sidebarExpanded` preference. 56 px collapsed gives the icon
// row enough breathing room (40 px tap target + 8 px each side);
// 200 px expanded matches the original sidebar width so existing
// layouts inside the sidebar (ServerSwitcher dropdown menus, nav
// labels) don't have to recalculate.
const SIDEBAR_W_COLLAPSED = 56;
const SIDEBAR_W_EXPANDED = 200;

function SidebarColumn({ children }: { children: ReactNode }) {
    const [expanded] = usePreference("ui.sidebarExpanded");
    const width = expanded ? SIDEBAR_W_EXPANDED : SIDEBAR_W_COLLAPSED;
    return (
        <div
            style={{
                flexShrink: 0,
                width,
                minWidth: width,
                maxWidth: width,
                height: "100%",
                display: "flex",
                flexDirection: "column",
                transition: "width 160ms ease-out",
            }}
        >
            {children}
        </div>
    );
}

// MainColumn is the right-hand pane that stacks the main content area
// on top of the global terminal drawer. The vertical ResizablePanelGroup
// owns the seam: dragging it grows / shrinks the drawer, and the
// drawer panel is sized imperatively in three regimes —
//   · no shells visible on this host  → 0 px (drawer hidden)
//   · drawer collapsed (Ctrl+`)        → TAB_BAR_HEIGHT (tab strip only)
//   · drawer open                      → drawerHeight (operator-chosen)
// drawerHeight is owned by GlobalTerminalContext (per-server localStorage)
// so we don't add a second persistence layer here; onResize feeds drag
// gestures straight back into setDrawerHeight.
function MainColumn({ children }: { children: ReactNode }) {
    const { shells, drawerOpen, drawerHeight, setDrawerHeight } = useGlobalTerminal();
    const { hostId: routeHostId } = useParams<{
        projectSlug?: string;
        hostId?: string;
        tab?: string;
    }>();
    const visibleShells = useMemo(
        () => shells.filter((s) => s.hostId === routeHostId),
        [shells, routeHostId],
    );
    const drawerActive = !!routeHostId && visibleShells.length > 0;

    const drawerPanelRef = usePanelRef();

    // Sync external drawer state (open / collapse / activate) back to
    // the panel's pixel size. We skip the resize() call when the panel
    // is already at the target so user-driven onResize ticks don't
    // rebound through this effect.
    useEffect(() => {
        const panel = drawerPanelRef.current;
        if (!panel) return;
        const targetPx = !drawerActive
            ? 0
            : drawerOpen
                ? drawerHeight
                : TAB_BAR_HEIGHT;
        const current = panel.getSize();
        if (Math.abs(current.inPixels - targetPx) <= 1) return;
        panel.resize(`${targetPx}px`);
    }, [drawerActive, drawerOpen, drawerHeight, drawerPanelRef]);

    return (
        <div
            style={{
                flex: 1,
                minWidth: 0,
                minHeight: 0,
                display: "flex",
                flexDirection: "column",
                // position: relative anchors the absolutely positioned
                // TransfersDrawer to the main column so it slides in
                // from the right edge of the outlet (not the viewport).
                position: "relative",
            }}
        >
            <ResizablePanelGroup
                direction="vertical"
                style={{ flex: 1, minHeight: 0 }}
            >
                <ResizablePanel id="main-content" minSize="20%" className="relative">
                    <main
                        style={{
                            position: "absolute",
                            inset: 0,
                            overflow: "auto",
                        }}
                    >
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
                </ResizablePanel>
                <ResizableHandle
                    disabled={!drawerActive || !drawerOpen}
                    className={!drawerActive ? "invisible" : undefined}
                />
                <ResizablePanel
                    id="terminal-drawer"
                    panelRef={drawerPanelRef}
                    defaultSize={
                        drawerActive
                            ? drawerOpen
                                ? `${drawerHeight}px`
                                : `${TAB_BAR_HEIGHT}px`
                            : "0px"
                    }
                    minSize="0px"
                    maxSize="85%"
                    className="relative"
                    onResize={(size) => {
                        // Only let the drag persist a new height when
                        // the drawer is meant to be open and active —
                        // otherwise toggle-induced resize() calls would
                        // overwrite the remembered open height with
                        // TAB_BAR_HEIGHT or 0.
                        if (drawerActive && drawerOpen && size.inPixels >= TAB_BAR_HEIGHT) {
                            setDrawerHeight(size.inPixels);
                        }
                    }}
                >
                    <div className="absolute inset-0">
                        <TerminalDrawer />
                    </div>
                </ResizablePanel>
            </ResizablePanelGroup>
            <TransfersDrawer />
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
