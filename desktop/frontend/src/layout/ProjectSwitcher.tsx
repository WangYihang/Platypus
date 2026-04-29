import { Check, Plus } from "lucide-react";
import { useNavigate } from "react-router-dom";

import Mono from "../components/Mono";
import { Project } from "../lib/api";
import { palette } from "./theme";

interface MenuProps {
    projects: Project[];
    currentSlug?: string;
    canCreateProject: boolean;
    onCreateProject?: () => void;
    onClose?: () => void;
}

// ProjectSwitcherMenu is the popover-content body shared by every
// project picker entry point — today the only consumer is TopBar's
// project breadcrumb, but Cmd-K's "Switch project" command can drop
// the same menu in front of the operator without rewriting the list /
// "All projects" / "New project" actions.
//
// The menu does not own a Popover wrapper — callers are expected to
// host this inside their own PopoverContent so the trigger styling
// stays close to the surrounding chrome (a breadcrumb pill, an
// inline button, etc.).
export function ProjectSwitcherMenu({
    projects,
    currentSlug,
    canCreateProject,
    onCreateProject,
    onClose,
}: MenuProps) {
    const navigate = useNavigate();
    return (
        <>
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
                            onClose?.();
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
                        onClose?.();
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
                            onClose?.();
                            onCreateProject();
                        }}
                    >
                        <Plus className="size-3.5 text-text-muted" />
                        <span>New project</span>
                    </button>
                )}
            </div>
        </>
    );
}
