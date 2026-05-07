import {
    ReactNode,
    Suspense,
    createContext,
    lazy,
    useCallback,
    useContext,
    useEffect,
    useState,
} from "react";
import { Outlet, useNavigate, useParams } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import EmptyState from "../components/EmptyState";
import StatusBar from "../components/StatusBar";
import { TransfersDrawer, TransfersDrawerProvider } from "../components/TransfersPill";
import { Project, createProject, listProjects } from "../lib/api";
import { humanizeError } from "../lib/humanizeError";
import { getSessionUser, getSession } from "../lib/auth";
import { listServers, setActiveServerId } from "../lib/servers";
import {
    GlobalTerminalProvider,
    useGlobalTerminal,
} from "../terminal/GlobalTerminalContext";
// CommandPalette + AddServerDialog mount but don't render contents
// until their `open` prop flips true — Cmd+K / "add server" click.
// Code-split them so the always-on shell bundle drops cmdk + the
// form stack + 12 lucide icons. Suspense fallback is null because
// closed dialogs render nothing anyway; the chunk fetches in
// parallel with the dialog's open animation on first use.
const CommandPalette = lazy(() => import("./CommandPalette"));
const AddServerDialog = lazy(() => import("./AddServerDialog"));
import NavTabs from "./NavTabs";
import TopBar from "./TopBar";
import { palette } from "./theme";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    Form,
    FormControl,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";

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
// :projectSlug → Project for nested routes, and renders the top-bar +
// nav-tabs + outlet vertical layout. The historical left rail was
// retired in the 2026-04 IA pass — see TopBar / NavTabs for the
// replacement chrome.
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
                        currentProject={project}
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
    currentProject,
    currentSlug,
    refresh,
    children,
}: {
    user: ReturnType<typeof getSessionUser> & {};
    serverURL: string;
    projects: Project[];
    currentProject: Project | null;
    currentSlug?: string;
    refresh: () => Promise<void>;
    children: ReactNode;
}) {
    const [addOpen, setAddOpen] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const navigate = useNavigate();
    // "Manage all…" used to open ManageServersDialog from inside the
    // shell. The dialog was promoted to /servers, so the action now
    // navigates instead. Same callback shape (`onManageServers`) keeps
    // every existing trigger (TopBar, ServerSwitcher, CommandPalette)
    // working without an API change.
    const goToServers = useCallback(() => navigate("/servers"), [navigate]);
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
            <TopBar
                user={user}
                serverURL={serverURL}
                projects={projects}
                currentProject={currentProject}
                onCreateProject={() => setCreateOpen(true)}
                onAddServer={() => setAddOpen(true)}
                onManageServers={goToServers}
            />
            <NavTabs user={user} currentSlug={currentSlug} />
            <div
                style={{
                    flex: 1,
                    minHeight: 0,
                    display: "flex",
                    flexDirection: "column",
                    overflow: "hidden",
                }}
            >
                <MainColumn>{children}</MainColumn>
            </div>
            <StatusBar />
            <Suspense fallback={null}>
                <CommandPalette
                    onAddServer={() => setAddOpen(true)}
                    onManageServers={goToServers}
                />
                <AddServerDialog open={addOpen} onOpenChange={setAddOpen} />
            </Suspense>
            <CreateProjectDialog
                open={createOpen}
                onOpenChange={setCreateOpen}
                onCreated={() => void refresh()}
            />
        </div>
    );
}

// CreateProjectDialog used to live inside ProjectSidebar. Hoisted to
// the shell so the "+ New project" entry point in TopBar's project
// breadcrumb can pop the same form without juggling lifted state.
function CreateProjectDialog({
    open,
    onOpenChange,
    onCreated,
}: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onCreated: () => void;
}) {
    const navigate = useNavigate();
    const projectSchema = z.object({
        name: z.string().min(1, "project name is required"),
        slug: z
            .string()
            .min(1, "slug is required")
            .regex(/^[a-z0-9][a-z0-9_-]{0,62}$/, {
                message: "a-z, 0-9, _ and - only; must start alphanumeric",
            }),
    });
    type FormValues = z.infer<typeof projectSchema>;
    const form = useForm<FormValues>({
        resolver: zodResolver(projectSchema),
        defaultValues: { name: "", slug: "" },
    });

    async function onSubmit(v: FormValues) {
        try {
            await createProject(v.name, v.slug);
            toast.success(`Created project ${v.slug}`);
            onOpenChange(false);
            form.reset({ name: "", slug: "" });
            onCreated();
            // Land inside the new project on Hosts — Overview at zero
            // hosts is uninformative; Hosts is the canonical "now
            // enrol your first agent" surface.
            navigate(`/projects/${v.slug}/hosts`);
        } catch (e) {
            toast.error(`create: ${humanizeError(e)}`);
        }
    }

    return (
        <Dialog
            open={open}
            onOpenChange={(o) => {
                onOpenChange(o);
                if (!o) form.reset({ name: "", slug: "" });
            }}
        >
            <DialogContent className="sm:max-w-[420px]">
                <DialogHeader>
                    <DialogTitle>New project</DialogTitle>
                    <DialogDescription>
                        Projects scope every resource (hosts, sessions, tokens).
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="name"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Project name</FormLabel>
                                    <FormControl>
                                        <Input autoFocus placeholder="Production" {...field} />
                                    </FormControl>
                                    <FormDescription>
                                        Human-friendly — shown in the top bar.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="slug"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Slug</FormLabel>
                                    <FormControl>
                                        <Input placeholder="prod" {...field} />
                                    </FormControl>
                                    <FormDescription>
                                        URL-safe id, unique across projects.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <DialogFooter>
                            <Button
                                type="button"
                                variant="outline"
                                onClick={() => onOpenChange(false)}
                            >
                                Cancel
                            </Button>
                            <Button type="submit" disabled={form.formState.isSubmitting}>
                                {form.formState.isSubmitting && (
                                    <Loader2 className="size-3.5 animate-spin" />
                                )}
                                Create
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}

// MainColumn renders the routed page inside a width-capped column
// and overlays the TransfersDrawer.
//
// The bottom terminal drawer used to live here too, but every shell
// is host-scoped — it now lives inside HostsShell so it only appears
// on `/hosts/*` routes. GlobalTerminalProvider stays at this level so
// open shells survive cross-tab navigation (open a shell, switch to
// /activity, come back — same shell, same scrollback).
function MainColumn({ children }: { children: ReactNode }) {
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
            <main style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
                {/* P3: cap the rendered content width on wide displays.
                    Without the cap, KPI cards, tables and member lists
                    stretched 1500+ px wide on 1920px monitors — long
                    lines are readable but tiring to scan. 1280px keeps
                    text rows in roughly the 80-100 character sweet spot
                    without leaving big black voids on either side at
                    narrower widths (the cap is a max, not a fixed
                    value). */}
                <div
                    data-testid="shell-content-frame"
                    style={{
                        maxWidth: 1280,
                        margin: "0 auto",
                        height: "100%",
                        display: "flex",
                        flexDirection: "column",
                    }}
                >
                    {children}
                </div>
            </main>
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
