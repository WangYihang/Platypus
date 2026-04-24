import { ReactNode, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import {
    Clock,
    CloudDownload,
    LayoutGrid,
    Loader2,
    Monitor,
    Settings2,
    Users,
} from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Brand from "../components/Brand";
import { Project, createProject } from "../lib/api";
import { SessionUser } from "../lib/auth";
import { palette, space } from "./theme";
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

    const createForm = useForm<ProjectFormValues>({
        resolver: zodResolver(projectSchema),
        defaultValues: { name: "", slug: "" },
    });

    const items: NavItem[] = [
        { to: "overview", label: "Overview", icon: <LayoutGrid className="size-4" />, requiresProject: true },
        { to: "fleet", label: "Fleet", icon: <Monitor className="size-4" />, requiresProject: true },
        { to: "activities", label: "Activities", icon: <Clock className="size-4" />, requiresProject: true },
        { to: "enrollment", label: "Enrollment", icon: <CloudDownload className="size-4" />, requiresProject: true, minRole: "admin" },
        { to: "members", label: "Members", icon: <Users className="size-4" />, requiresProject: true, minRole: "operator" },
        { to: "settings", label: "Settings", icon: <Settings2 className="size-4" />, requiresProject: true, minRole: "admin" },
    ];

    const visible = items.filter((it) => meetsRole(user.role, it.minRole));

    async function handleCreateProject(v: ProjectFormValues) {
        try {
            await createProject(v.name, v.slug);
            toast.success(`Created project ${v.slug}`);
            setCreateOpen(false);
            createForm.reset({ name: "", slug: "" });
            onProjectsChanged();
        } catch (e) {
            toast.error(`create: ${String(e)}`);
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
                            className={({ isActive }) => {
                                // Host detail pages live at /hosts/:id/:tab
                                // but conceptually belong under Fleet, so
                                // highlight Fleet there too.
                                const forced =
                                    it.to === "fleet" &&
                                    pathname.startsWith(`/projects/${currentSlug}/hosts/`);
                                const active = isActive || forced;
                                return "pl-nav-link" + (active ? " pl-nav-link--active" : "");
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
