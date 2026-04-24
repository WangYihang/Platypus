import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";
import { Outlet, useParams } from "react-router-dom";
import { Loader2 } from "lucide-react";

import EmptyState from "../components/EmptyState";
import StatusBar from "../components/StatusBar";
import { Project, listProjects } from "../lib/api";
import { getSessionUser, getSession } from "../lib/auth";
import {
    GlobalTerminalProvider,
    useGlobalTerminal,
} from "../terminal/GlobalTerminalContext";
import TerminalDrawer from "../terminal/TerminalDrawer";
import CommandPalette from "./CommandPalette";
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

    if (projects === null) {
        return (
            <ShellChrome
                user={user}
                serverURL={serverURL}
                projects={[]}
                currentSlug={params.projectSlug}
                refresh={refresh}
            >
                <Centered>
                    <Loader2 className="size-5 animate-spin text-text-muted" />
                </Centered>
            </ShellChrome>
        );
    }

    const project = params.projectSlug
        ? projects.find((p) => p.slug === params.projectSlug) ?? null
        : null;

    return (
        <ProjectShellContext.Provider value={{ projects, project, refresh }}>
            <GlobalTerminalProvider>
                <ShellChrome
                    user={user}
                    serverURL={serverURL}
                    projects={projects}
                    currentSlug={params.projectSlug}
                    refresh={refresh}
                >
                    {requireProject && !project ? (
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
    useGlobalTerminalHotkey();
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
            <CommandPalette />
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
