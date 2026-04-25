import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useNavigate } from "react-router-dom";
import {
    Clock,
    CloudDownload,
    CornerDownLeft,
    FolderKanban,
    LayoutGrid,
    Monitor,
    Plus,
    Search,
    Server,
    Settings2,
    TerminalSquare,
    Users,
} from "lucide-react";
import {
    CommandDialog,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
    CommandSeparator,
} from "cmdk";

import { palette, radius, space } from "./theme";
import { useShell } from "./ProjectShell";
import { Host, listHosts } from "../lib/api";
import {
    ServerProfile,
    getActiveServerId,
    listServers,
    onServersChange,
} from "../lib/servers";
import { switchServer } from "../lib/auth";
import { useGlobalTerminal } from "../terminal/GlobalTerminalContext";

interface Props {
    onAddServer?: () => void;
    onManageServers?: () => void;
}

// CommandPalette is the keyboard-first nav surface (Cmd/Ctrl+K). It
// lives at the shell level alongside the global terminal provider so
// it's always reachable. Content is scoped to the current project:
// page nav, project switch, host navigation, opening a shell on any
// host, switching between saved servers.
export default function CommandPalette({ onAddServer, onManageServers }: Props) {
    const navigate = useNavigate();
    const { project, projects } = useShell();
    const { openShell } = useGlobalTerminal();
    const [open, setOpen] = useState(false);
    const [hosts, setHosts] = useState<Host[]>([]);
    const [loadingHosts, setLoadingHosts] = useState(false);
    const servers = useServerList();
    const activeServerId = useActiveServerId();

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
                e.preventDefault();
                setOpen((v) => !v);
            }
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);

    // Hosts are only useful when a project is active; fetch lazily on
    // each open so freshly-enrolled hosts show up without a page
    // reload.
    useEffect(() => {
        if (!open || !project) return;
        let cancelled = false;
        setLoadingHosts(true);
        listHosts(project.id)
            .then((hs) => {
                if (!cancelled) setHosts(hs);
            })
            .catch(() => {
                if (!cancelled) setHosts([]);
            })
            .finally(() => {
                if (!cancelled) setLoadingHosts(false);
            });
        return () => {
            cancelled = true;
        };
    }, [open, project]);

    const run = useCallback(
        (fn: () => void) => {
            setOpen(false);
            // Defer so CommandDialog's unmount doesn't race the action
            // (e.g. navigate() triggering a focus flip while the dialog
            // is still tearing down).
            setTimeout(fn, 0);
        },
        [],
    );

    const go = (rel: string) => {
        if (!project) return;
        run(() => navigate(`/projects/${project.slug}/${rel}`));
    };

    return (
        <CommandDialog
            open={open}
            onOpenChange={setOpen}
            label="Command palette"
            // cmdk's <Dialog /> renders a Radix Dialog; pass inline
            // styles on the inner <Command /> content to colour-match
            // the shell theme.
            contentClassName="pl-cmdk-content"
            overlayClassName="pl-cmdk-overlay"
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `0 ${space[3]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <Search className="size-4" style={{ color: palette.textMuted }} />
                <CommandInput
                    placeholder={
                        project
                            ? "Jump to page, host, or project…"
                            : "Switch project…"
                    }
                    style={{
                        flex: 1,
                        height: 44,
                        border: "none",
                        outline: "none",
                        background: "transparent",
                        color: palette.textPrimary,
                        fontSize: 14,
                        fontFamily: "var(--font-geist-sans)",
                    }}
                />
                <Kbd>Esc</Kbd>
            </div>

            <CommandList
                style={{
                    maxHeight: 360,
                    overflow: "auto",
                    padding: space[2],
                }}
            >
                <CommandEmpty
                    style={{
                        padding: space[5],
                        color: palette.textMuted,
                        textAlign: "center",
                        fontSize: 13,
                    }}
                >
                    No matches.
                </CommandEmpty>

                {project && (
                    <CommandGroup heading="Navigate">
                        <PaletteItem
                            icon={<LayoutGrid className="size-4" />}
                            label="Overview"
                            shortcut="overview"
                            onSelect={() => go("overview")}
                        />
                        <PaletteItem
                            icon={<Monitor className="size-4" />}
                            label="Fleet"
                            shortcut="hosts sessions topology"
                            onSelect={() => go("fleet")}
                        />
                        <PaletteItem
                            icon={<Clock className="size-4" />}
                            label="Activities"
                            shortcut="activity log"
                            onSelect={() => go("activities")}
                        />
                        <PaletteItem
                            icon={<CloudDownload className="size-4" />}
                            label="Enrollment"
                            shortcut="pat install artifacts"
                            onSelect={() => go("enrollment")}
                        />
                        <PaletteItem
                            icon={<Users className="size-4" />}
                            label="Members"
                            shortcut="team"
                            onSelect={() => go("members")}
                        />
                        <PaletteItem
                            icon={<Settings2 className="size-4" />}
                            label="Settings"
                            shortcut="project"
                            onSelect={() => go("settings")}
                        />
                    </CommandGroup>
                )}

                {project && hosts.length > 0 && (
                    <>
                        <CommandSeparator
                            style={{
                                height: 1,
                                background: palette.border,
                                margin: `${space[2]}px 0`,
                            }}
                        />
                        <CommandGroup heading="Hosts">
                            {hosts.map((h) => {
                                const label =
                                    h.primary_alias ||
                                    h.hostname ||
                                    h.machine_id?.slice(0, 8) ||
                                    "unknown";
                                const keywords = [
                                    h.hostname,
                                    h.primary_alias,
                                    h.os,
                                    h.platform,
                                    h.machine_id,
                                    h.primary_ip,
                                ]
                                    .filter(Boolean)
                                    .map(String);
                                return (
                                    <PaletteItem
                                        key={`host-${h.id}`}
                                        value={`host ${h.id} ${label}`}
                                        icon={<Monitor className="size-4" />}
                                        label={label}
                                        hint={h.os || h.primary_ip || ""}
                                        keywords={keywords}
                                        onSelect={() =>
                                            run(() =>
                                                navigate(
                                                    `/projects/${project.slug}/hosts/${h.id}/info`,
                                                ),
                                            )
                                        }
                                    />
                                );
                            })}
                        </CommandGroup>
                        <CommandGroup heading="Open shell on…">
                            {hosts
                                .filter((h) => !!h.agent_id)
                                .map((h) => {
                                    const label =
                                        h.primary_alias ||
                                        h.hostname ||
                                        h.machine_id?.slice(0, 8) ||
                                        "unknown";
                                    return (
                                        <PaletteItem
                                            key={`term-${h.id}`}
                                            value={`terminal ${h.id} ${label}`}
                                            icon={<TerminalSquare className="size-4" />}
                                            label={label}
                                            hint="Open in bottom panel"
                                            keywords={[label, "shell", "terminal"]}
                                            onSelect={() =>
                                                run(() => {
                                                    if (!h.agent_id) return;
                                                    openShell({
                                                        projectID: project.id,
                                                        projectSlug: project.slug,
                                                        hostId: h.id,
                                                        sessionHash: h.agent_id,
                                                        label,
                                                    });
                                                })
                                            }
                                        />
                                    );
                                })}
                        </CommandGroup>
                    </>
                )}

                {loadingHosts && project && hosts.length === 0 && (
                    <CommandGroup heading="Hosts">
                        <div style={{ padding: space[3], color: palette.textMuted, fontSize: 12 }}>
                            Loading hosts…
                        </div>
                    </CommandGroup>
                )}

                {projects.length > 1 && (
                    <>
                        <CommandSeparator
                            style={{
                                height: 1,
                                background: palette.border,
                                margin: `${space[2]}px 0`,
                            }}
                        />
                        <CommandGroup heading="Switch project">
                            {projects.map((p) => (
                                <PaletteItem
                                    key={`proj-${p.id}`}
                                    value={`project ${p.slug} ${p.name}`}
                                    icon={<FolderKanban className="size-4" />}
                                    label={p.name}
                                    hint={p.slug}
                                    keywords={[p.slug, p.name]}
                                    onSelect={() =>
                                        run(() => navigate(`/projects/${p.slug}/overview`))
                                    }
                                />
                            ))}
                        </CommandGroup>
                    </>
                )}

                <CommandSeparator
                    style={{
                        height: 1,
                        background: palette.border,
                        margin: `${space[2]}px 0`,
                    }}
                />
                <CommandGroup heading="Servers">
                    {servers
                        .filter((s) => s.id !== activeServerId)
                        .map((s) => (
                            <PaletteItem
                                key={`srv-${s.id}`}
                                value={`server ${s.name} ${s.url}`}
                                icon={<Server className="size-4" />}
                                label={`Switch to ${s.name}`}
                                hint={s.url}
                                keywords={[s.name, s.url]}
                                onSelect={() => run(() => void switchServer(s.id))}
                            />
                        ))}
                    {onAddServer && (
                        <PaletteItem
                            icon={<Plus className="size-4" />}
                            label="Add server…"
                            keywords={["new", "add", "server"]}
                            onSelect={() => run(() => onAddServer())}
                        />
                    )}
                    {onManageServers && (
                        <PaletteItem
                            icon={<Settings2 className="size-4" />}
                            label="Manage servers…"
                            keywords={["manage", "servers", "rename", "remove"]}
                            onSelect={() => run(() => onManageServers())}
                        />
                    )}
                </CommandGroup>
            </CommandList>

            <div
                style={{
                    display: "flex",
                    justifyContent: "flex-end",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[2]}px ${space[3]}px`,
                    borderTop: `1px solid ${palette.border}`,
                    color: palette.textMuted,
                    fontSize: 11,
                }}
            >
                <Kbd>
                    <CornerDownLeft className="size-3" />
                </Kbd>
                <span>to run</span>
            </div>
        </CommandDialog>
    );
}

interface PaletteItemProps {
    value?: string;
    icon: React.ReactNode;
    label: string;
    hint?: string;
    shortcut?: string;
    keywords?: string[];
    onSelect: () => void;
}

function PaletteItem({
    value,
    icon,
    label,
    hint,
    shortcut,
    keywords,
    onSelect,
}: PaletteItemProps) {
    return (
        <CommandItem
            value={value ?? `${label} ${shortcut ?? ""}`}
            keywords={keywords}
            onSelect={onSelect}
            className="pl-cmdk-item"
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[3],
                padding: `${space[2]}px ${space[3]}px`,
                borderRadius: radius.sm,
                cursor: "pointer",
                color: palette.textPrimary,
                fontSize: 13,
            }}
        >
            <span style={{ color: palette.textSecondary }}>{icon}</span>
            <span style={{ flex: 1 }}>{label}</span>
            {hint && (
                <span style={{ color: palette.textMuted, fontSize: 12 }}>{hint}</span>
            )}
        </CommandItem>
    );
}

// servers.ts fires onServersChange on both mutations and active
// pointer changes, so one version counter covers both reads.
let serverVersion = 0;
onServersChange(() => {
    serverVersion++;
});

function useServerList(): ServerProfile[] {
    const v = useSyncExternalStore(
        (fn) => onServersChange(fn),
        () => serverVersion,
        () => serverVersion,
    );
    return useMemo(() => listServers(), [v]);
}

function useActiveServerId(): string | null {
    return useSyncExternalStore(
        (fn) => onServersChange(fn),
        () => getActiveServerId(),
        () => getActiveServerId(),
    );
}

function Kbd({ children }: { children: React.ReactNode }) {
    return (
        <span
            style={{
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                padding: "1px 6px",
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.sm,
                color: palette.textSecondary,
                fontSize: 11,
                fontFamily: "var(--font-geist-mono)",
                lineHeight: 1.4,
            }}
        >
            {children}
        </span>
    );
}
