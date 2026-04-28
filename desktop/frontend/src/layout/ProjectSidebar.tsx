import { ReactNode, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { Loader2 } from "lucide-react";

import { icons } from "../lib/icons";
import { toast } from "sonner";
import { humanizeError } from "../lib/humanizeError";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Brand from "../components/Brand";
import { Project, createProject } from "../lib/api";
import { SessionUser } from "../lib/auth";
import { palette, space } from "./theme";
import CmdKHint from "./CmdKHint";
import ProjectSwitcher from "./ProjectSwitcher";
import UserMenu from "./UserMenu";

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

const projectSchema = z.object({
    name: z.string().min(1, "project name is required"),
    slug: z
        .string()
        .min(1, "slug is required")
        .regex(/^[a-z0-9][a-z0-9_-]{0,62}$/, {
            message: "a-z, 0-9, _ and - only; must start alphanumeric",
        }),
});
type ProjectFormValues = z.infer<typeof projectSchema>;

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

// NavGroup splits the per-project nav into four IA buckets so new
// users read the relationship at a glance:
//
//   work    — daily-use surfaces: Overview / Fleet (Enroll lives inside)
//   admin   — access control: Members
//   audit   — read-only history: Activities (and future audit views)
//   project — project-level configuration: Settings
//
// Enrollment used to be a top-level Admin item, but the tokens it
// issues are one-shot agent-bootstrap secrets, not account-level
// credentials. Conceptually it's "how you grow the fleet" — so the
// surface lives inside FleetPage and is reached via an "Enroll
// agent" entry point in the Fleet header.
//
// Group order, labels, and item ordering are pinned by
// e2e/specs/56-sidebar-nav-grouping.spec.ts so a future "let me
// reorder these" can't silently put Settings above Fleet.
type NavGroupKey = "work" | "admin" | "audit" | "project";

interface NavGroup {
    key: NavGroupKey;
    label: string;
    items: NavItem[];
}

// ProjectSidebar is the left rail. Linear/Resend-style: brand at top,
// project switcher dropdown, flat per-project nav, user menu pinned at
// the bottom.
export default function ProjectSidebar({
    user,
    serverURL,
    projects,
    currentSlug,
    onProjectsChanged,
}: Props) {
    const [createOpen, setCreateOpen] = useState(false);
    const { pathname } = useLocation();
    const navigate = useNavigate();

    const createForm = useForm<ProjectFormValues>({
        resolver: zodResolver(projectSchema),
        defaultValues: { name: "", slug: "" },
    });

    // Every nav item's icon comes from lib/icons.ts so the same
    // domain noun always renders with the same glyph everywhere it
    // appears (sidebar, page header, empty-state, etc.). Don't reach
    // into lucide-react directly here — extend the registry instead.
    const I = icons;
    const groups: NavGroup[] = [
        {
            key: "work",
            label: "Work",
            items: [
                { to: "overview", label: "Overview", icon: <I.project className="size-4" />, requiresProject: true },
                { to: "fleet", label: "Fleet", icon: <I.fleet className="size-4" />, requiresProject: true },
            ],
        },
        {
            key: "admin",
            label: "Admin",
            items: [
                { to: "members", label: "Members", icon: <I.members className="size-4" />, requiresProject: true, minRole: "operator" },
            ],
        },
        {
            key: "audit",
            label: "Audit",
            items: [
                { to: "activities", label: "Activities", icon: <I.activity className="size-4" />, requiresProject: true },
                { to: "recordings", label: "Recordings", icon: <I.recordings className="size-4" />, requiresProject: true },
                { to: "transfers", label: "Transfers", icon: <I.transfers className="size-4" />, requiresProject: true },
            ],
        },
        {
            key: "project",
            label: "Project",
            items: [
                { to: "settings", label: "Settings", icon: <I.settings className="size-4" />, requiresProject: true, minRole: "admin" },
            ],
        },
    ];

    const visibleGroups = groups
        .map((g) => ({
            ...g,
            items: g.items.filter((it) => meetsRole(user.role, it.minRole)),
        }))
        .filter((g) => g.items.length > 0);

    async function handleCreateProject(v: ProjectFormValues) {
        try {
            await createProject(v.name, v.slug);
            toast.success(`Created project ${v.slug}`);
            setCreateOpen(false);
            createForm.reset({ name: "", slug: "" });
            // Refresh the projects list first so the freshly-created
            // project is in the shell context the next route renders
            // against — otherwise navigating to /fleet immediately
            // would hit the "select a project" empty path.
            onProjectsChanged();
            // Drop the user inside the project they just made, on
            // Fleet (not Overview): with zero hosts the Overview is a
            // KPI dashboard of zeros, while Fleet is the canonical
            // "now enrol your first agent" surface.
            navigate(`/projects/${v.slug}/fleet`);
        } catch (e) {
            toast.error(`create: ${humanizeError(e)}`);
        }
    }

    return (
        <aside
            style={{
                width: "100%",
                height: "100%",
                minWidth: 0,
                background: palette.sidebar,
                borderRight: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[3]}px ${space[3]}px ${space[2]}px`,
                }}
            >
                <Brand />
                <span
                    style={{
                        fontWeight: 600,
                        color: palette.textPrimary,
                        fontSize: 13,
                        letterSpacing: -0.2,
                        flex: 1,
                    }}
                >
                    Platypus
                </span>
                <CmdKHint />
            </div>

            <div style={{ padding: `0 ${space[3]}px ${space[2]}px` }}>
                <ProjectSwitcher
                    projects={projects}
                    currentSlug={currentSlug}
                    canCreateProject={user.role === "admin"}
                    onCreateProject={() => setCreateOpen(true)}
                />
            </div>

            <nav style={{ flex: 1, padding: `${space[1]}px ${space[2]}px`, overflow: "auto" }}>
                {currentSlug ? (
                    visibleGroups.map((g, gi) => (
                        <div
                            key={g.key}
                            style={{
                                // First group sits flush; subsequent
                                // groups get extra top space so the
                                // section break reads as a break, not
                                // an accidental gap. Tightened from
                                // space[3] (12) → space[2] (8) so all
                                // four IA groups + their items fit
                                // above the user menu without scroll.
                                marginTop: gi === 0 ? 0 : space[2],
                            }}
                        >
                            <div
                                data-testid={`nav-group-${g.key}`}
                                style={{
                                    padding: `${space[1]}px ${space[3]}px 2px`,
                                    fontSize: 10,
                                    fontWeight: 600,
                                    letterSpacing: 0.6,
                                    textTransform: "uppercase",
                                    color: palette.textMuted,
                                }}
                            >
                                {g.label}
                            </div>
                            <div data-testid={`nav-group-items-${g.key}`}>
                                {g.items.map((it) => (
                                    <NavLink
                                        key={it.to}
                                        to={`/projects/${currentSlug}/${it.to}`}
                                        className={({ isActive }) => {
                                            // Host detail pages live at
                                            // /hosts/:id/:tab but
                                            // conceptually belong under
                                            // Fleet, so highlight Fleet
                                            // there too.
                                            const forced =
                                                it.to === "fleet" &&
                                                pathname.startsWith(
                                                    `/projects/${currentSlug}/hosts/`,
                                                );
                                            const active = isActive || forced;
                                            return (
                                                "pl-nav-link" +
                                                (active ? " pl-nav-link--active" : "")
                                            );
                                        }}
                                    >
                                        <span
                                            style={{
                                                width: 16,
                                                display: "inline-flex",
                                                justifyContent: "center",
                                            }}
                                        >
                                            {it.icon}
                                        </span>
                                        <span>{it.label}</span>
                                    </NavLink>
                                ))}
                            </div>
                        </div>
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
                        Pick a project to see its hosts and sessions.
                    </div>
                )}
            </nav>

            <UserMenu user={user} serverURL={serverURL} />

            <Dialog
                open={createOpen}
                onOpenChange={(o) => {
                    setCreateOpen(o);
                    if (!o) createForm.reset({ name: "", slug: "" });
                }}
            >
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>New project</DialogTitle>
                        <DialogDescription>
                            Projects scope every resource (hosts, sessions, tokens).
                        </DialogDescription>
                    </DialogHeader>
                    <Form {...createForm}>
                        <form
                            onSubmit={createForm.handleSubmit(handleCreateProject)}
                            className="space-y-4"
                        >
                            <FormField
                                control={createForm.control}
                                name="name"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Project name</FormLabel>
                                        <FormControl>
                                            <Input autoFocus placeholder="Production" {...field} />
                                        </FormControl>
                                        <FormDescription>
                                            Human-friendly — shown in the sidebar header.
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={createForm.control}
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
                                    onClick={() => setCreateOpen(false)}
                                >
                                    Cancel
                                </Button>
                                <Button
                                    type="submit"
                                    disabled={createForm.formState.isSubmitting}
                                >
                                    {createForm.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Create
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>
        </aside>
    );
}

function meetsRole(actual: SessionUser["role"], required?: SessionUser["role"]): boolean {
    if (!required) return true;
    const order = { viewer: 0, operator: 1, admin: 2 };
    return order[actual] >= order[required];
}
