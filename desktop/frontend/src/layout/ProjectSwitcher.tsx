import { useState } from "react";
import { Check, ChevronDown, Plus } from "lucide-react";
import { useNavigate } from "react-router-dom";

import Mono from "../components/Mono";
import { Project } from "../lib/api";
import { palette, radius, space } from "./theme";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

interface Props {
    projects: Project[];
    currentSlug?: string;
    canCreateProject: boolean;
    onCreateProject?: () => void;
}

// ProjectSwitcher is the top-of-sidebar dropdown. Displays the current
// project (name + slug) + chevron; clicking opens a popover listing
// every project + footer links ("All projects" / "New project" for
// admins). Picking a project navigates to its overview.
export default function ProjectSwitcher({
    projects,
    currentSlug,
    canCreateProject,
    onCreateProject,
}: Props) {
    const [open, setOpen] = useState(false);
    const navigate = useNavigate();
    const current = projects.find((p) => p.slug === currentSlug);

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    data-testid="project-switcher-trigger"
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: space[2],
                        width: "100%",
                        padding: `${space[2]}px ${space[3]}px`,
                        background: palette.surfaceHover,
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.md,
                        color: palette.textPrimary,
                        cursor: "pointer",
                        textAlign: "left",
                        fontSize: 13,
                    }}
                >
                    <span style={{ flex: 1, minWidth: 0, overflow: "hidden" }}>
                        {current ? (
                            <>
                                <div
                                    style={{
                                        fontWeight: 600,
                                        overflow: "hidden",
                                        textOverflow: "ellipsis",
                                        whiteSpace: "nowrap",
                                    }}
                                >
                                    {current.name}
                                </div>
                                <Mono size={11} color={palette.textMuted}>
                                    {current.slug}
                                </Mono>
                            </>
                        ) : (
                            <div style={{ color: palette.textSecondary }}>All projects</div>
                        )}
                    </span>
                    <ChevronDown className="size-3 text-text-muted" />
                </button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-[260px] p-1">
                <div
                    className="px-3 py-2 text-[11px] uppercase text-text-muted"
                    style={{ letterSpacing: 0.5, fontWeight: 600 }}
                >
                    Projects
                </div>
                <div style={{ maxHeight: 280, overflow: "auto" }}>
                    {projects.map((p) => (
                        <button
                            key={p.id}
                            type="button"
                            className="pl-popover-btn"
                            onClick={() => {
                                setOpen(false);
                                navigate(`/projects/${p.slug}/overview`);
                            }}
                        >
                            <span style={{ flex: 1, minWidth: 0 }}>
                                <span
                                    style={{
                                        color: palette.textPrimary,
                                        fontWeight: 500,
                                        display: "block",
                                    }}
                                >
                                    {p.name}
                                </span>
                                <Mono size={11} color={palette.textMuted}>
                                    {p.slug}
                                </Mono>
                            </span>
                            {p.slug === currentSlug && (
                                <Check className="size-3.5 text-text-primary" />
                            )}
                        </button>
                    ))}
                </div>
                <div className="mt-2 pt-2 border-t border-border">
                    <button
                        type="button"
                        className="pl-popover-btn"
                        onClick={() => {
                            setOpen(false);
                            navigate("/projects");
                        }}
                    >
                        <span style={{ color: palette.textSecondary }}>All projects</span>
                    </button>
                    {canCreateProject && onCreateProject && (
                        <button
                            type="button"
                            className="pl-popover-btn"
                            onClick={() => {
                                setOpen(false);
                                onCreateProject();
                            }}
                        >
                            <Plus className="size-3.5 text-text-muted" />
                            <span>New project</span>
                        </button>
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
