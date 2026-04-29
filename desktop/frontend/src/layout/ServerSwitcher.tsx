import { useCallback, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ChevronDown, Plus, Settings } from "lucide-react";
import { DndContext, DragEndEvent } from "@dnd-kit/core";
import {
    SortableContext,
    arrayMove,
    verticalListSortingStrategy,
} from "@dnd-kit/sortable";

import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import { useDragSensors } from "../lib/dnd";
import { radius } from "./theme";
import {
    ServerProfile,
    avatarFor,
    reorderServers,
} from "../lib/servers";
import { switchServer } from "../lib/auth";

import SortableServerRow from "./server-switcher/SortableServerRow";
import {
    useActiveServer,
    useActiveServerId,
    useServerList,
} from "./server-switcher/hooks";

interface Props {
    onAddServer: () => void;
    onManageServers: () => void;
}

// ServerSwitcher is the consolidated server picker that replaced the
// old 64 px ServerRail column. Mounts at the top of ProjectSidebar:
// trigger pill at the top, dropdown lists every saved server with
// drag-to-reorder, per-row actions (rename / sign-out / remove), plus
// "Add server…" and "Manage all…" entries at the bottom.
export default function ServerSwitcher({ onAddServer, onManageServers }: Props) {
    const profiles = useServerList();
    const activeId = useActiveServerId();
    const active = useActiveServer();
    const navigate = useNavigate();
    const location = useLocation();
    const [open, setOpen] = useState(false);

    const sensors = useDragSensors();

    const handleDragEnd = (e: DragEndEvent) => {
        const { active: from, over } = e;
        if (!over || from.id === over.id) return;
        const ids = profiles.map((p) => p.id);
        const fromIdx = ids.indexOf(String(from.id));
        const toIdx = ids.indexOf(String(over.id));
        if (fromIdx < 0 || toIdx < 0) return;
        reorderServers(arrayMove(ids, fromIdx, toIdx));
    };

    const onRowClick = useCallback(
        async (profile: ServerProfile) => {
            setOpen(false);
            const { loggedIn } = await switchServer(profile.id);
            if (loggedIn) return;
            navigate("/login", {
                state: {
                    serverId: profile.id,
                    serverURL: profile.url,
                    from: location,
                },
            });
        },
        [navigate, location],
    );

    const activeAvatar = active ? avatarFor(active) : null;

    return (
        <DropdownMenu open={open} onOpenChange={setOpen}>
            <DropdownMenuTrigger asChild>
                <button
                    type="button"
                    data-testid="server-switcher-trigger"
                    aria-label={active ? `Active server: ${active.name}` : "Select server"}
                    className="flex w-full items-center gap-2 rounded-md border border-border-subtle bg-surface px-2 py-1 text-left text-xs hover:bg-surface-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                    {activeAvatar ? (
                        <span
                            aria-hidden
                            style={{
                                width: 20,
                                height: 20,
                                borderRadius: radius.sm,
                                background: activeAvatar.bg,
                                color: activeAvatar.fg,
                                display: "inline-flex",
                                alignItems: "center",
                                justifyContent: "center",
                                fontSize: 11,
                                fontWeight: 600,
                                flexShrink: 0,
                            }}
                        >
                            {activeAvatar.letter}
                        </span>
                    ) : (
                        <span
                            aria-hidden
                            className="size-5 shrink-0 rounded-sm border border-dashed border-border-subtle"
                        />
                    )}
                    <span className="min-w-0 flex-1 truncate font-medium text-text-primary">
                        {active?.name ?? "No server"}
                    </span>
                    <ChevronDown className="size-3.5 shrink-0 text-text-muted" />
                </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
                align="start"
                sideOffset={4}
                className="w-[280px] p-1"
                data-testid="server-switcher-menu"
            >
                <DropdownMenuLabel className="px-2 py-1 text-[10px] font-medium uppercase tracking-wider text-text-muted">
                    Servers
                </DropdownMenuLabel>
                <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
                    <SortableContext
                        items={profiles.map((p) => p.id)}
                        strategy={verticalListSortingStrategy}
                    >
                        <div className="flex flex-col gap-0.5">
                            {profiles.map((p, i) => (
                                <SortableServerRow
                                    key={p.id}
                                    profile={p}
                                    index={i}
                                    active={p.id === activeId}
                                    onActivate={() => void onRowClick(p)}
                                    onCloseMenu={() => setOpen(false)}
                                />
                            ))}
                        </div>
                    </SortableContext>
                </DndContext>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                    onSelect={() => {
                        setOpen(false);
                        onAddServer();
                    }}
                    data-testid="server-switcher-add"
                    className="gap-2"
                >
                    <Plus className="size-3.5" />
                    Add server…
                </DropdownMenuItem>
                <DropdownMenuItem
                    onSelect={() => {
                        setOpen(false);
                        onManageServers();
                    }}
                    data-testid="server-switcher-manage"
                    className="gap-2"
                >
                    <Settings className="size-3.5" />
                    Manage all…
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
