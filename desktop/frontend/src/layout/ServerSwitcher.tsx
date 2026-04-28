import {
    CSSProperties,
    useCallback,
    useEffect,
    useMemo,
    useState,
    useSyncExternalStore,
} from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
    ChevronDown,
    GripVertical,
    LogOut,
    Pencil,
    Plus,
    Settings,
    Trash2,
} from "lucide-react";
import {
    DndContext,
    DragEndEvent,
} from "@dnd-kit/core";
import {
    SortableContext,
    arrayMove,
    useSortable,
    verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS as DndCSS } from "@dnd-kit/utilities";

import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";

import { useDragSensors } from "../lib/dnd";
import { palette, radius } from "./theme";
import {
    ServerProfile,
    avatarFor,
    getActiveServerId,
    listServers,
    onServersChange,
    renameServer,
    reorderServers,
} from "../lib/servers";
import {
    ConnectionState,
    connectionState as connState,
    onConnectionStateChange,
} from "../lib/notify";
import {
    forgetAndRemoveServer,
    forgetServer,
    hasPersistedSession,
    onSessionChange,
    switchServer,
} from "../lib/auth";
import { useUnread } from "../lib/unread";

interface Props {
    onAddServer: () => void;
    onManageServers: () => void;
}

// ServerSwitcher is the consolidated server picker that replaced the
// old 64 px ServerRail column. It mounts at the top of ProjectSidebar
// as a single full-width button — clicking opens a dropdown listing
// every saved profile. Each row is drag-sortable (same dnd-kit
// hooks), shows the connection dot + unread badge inline, and
// exposes per-row rename / sign-out / remove actions on hover so
// CRUD doesn't require opening "Manage all". Switching by keyboard
// (Ctrl+1..9 / Cmd+1..9) still flows through useServerSwitchHotkeys
// in ProjectShell — that hook reads listServers() directly, so
// dropping the rail-based DOM didn't change the contract.
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

interface RowProps {
    profile: ServerProfile;
    index: number;
    active: boolean;
    onActivate: () => void;
    onCloseMenu: () => void;
}

function SortableServerRow({ profile, index, active, onActivate, onCloseMenu }: RowProps) {
    const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
        useSortable({ id: profile.id });
    const avatar = avatarFor(profile);
    const state = useConnectionState(profile.id);
    const unread = useUnread(profile.id);
    const loggedIn = useHasPersistedSession(profile.id);
    const dotColor = dotColorForState(state, loggedIn);

    const [renameOpen, setRenameOpen] = useState(false);
    const [renameValue, setRenameValue] = useState(profile.name);
    const [removeOpen, setRemoveOpen] = useState(false);

    const commitRename = () => {
        const next = renameValue.trim();
        if (next && next !== profile.name) {
            renameServer(profile.id, next);
        }
        setRenameOpen(false);
    };

    const shortcut =
        index < 9
            ? (typeof navigator !== "undefined" && /Mac/i.test(navigator.platform)
                  ? "⌘"
                  : "Ctrl") +
              (index + 1)
            : null;

    const rowStyle: CSSProperties = {
        transform: DndCSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
    };

    return (
        <>
            <div
                ref={setNodeRef}
                data-testid={`server-row-${index}`}
                data-active={active || undefined}
                className={cn(
                    "group flex items-center gap-2 rounded-md px-1.5 py-1 text-xs",
                    active && "bg-accent",
                    !active && "hover:bg-accent/60",
                )}
                style={rowStyle}
            >
                <button
                    type="button"
                    aria-label="Drag to reorder"
                    {...attributes}
                    {...listeners}
                    className="flex size-4 cursor-grab items-center justify-center text-text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                    onClick={(e) => e.stopPropagation()}
                >
                    <GripVertical className="size-3.5" />
                </button>
                <button
                    type="button"
                    onClick={onActivate}
                    aria-label={`Switch to ${profile.name}`}
                    className="flex min-w-0 flex-1 items-center gap-2 text-left"
                >
                    <span
                        aria-hidden
                        style={{
                            position: "relative",
                            width: 22,
                            height: 22,
                            borderRadius: radius.sm,
                            background: avatar.bg,
                            color: avatar.fg,
                            display: "inline-flex",
                            alignItems: "center",
                            justifyContent: "center",
                            fontSize: 11,
                            fontWeight: 600,
                            flexShrink: 0,
                        }}
                    >
                        {avatar.letter}
                        <span
                            aria-hidden
                            style={{
                                position: "absolute",
                                right: -2,
                                bottom: -2,
                                width: 8,
                                height: 8,
                                borderRadius: 999,
                                background: dotColor,
                                border: `1.5px solid ${palette.surface}`,
                            }}
                        />
                    </span>
                    <span className="flex min-w-0 flex-1 flex-col leading-tight">
                        <span className="flex items-center gap-1.5 truncate">
                            <span
                                data-testid={
                                    active ? "rail-active-indicator" : undefined
                                }
                                className="truncate font-medium text-text-primary"
                            >
                                {profile.name}
                            </span>
                            {unread > 0 && !active && (
                                <span
                                    aria-label={`${unread} unread`}
                                    className="rounded-full bg-danger px-1 text-[9px] font-semibold text-white"
                                    style={{ background: palette.danger }}
                                >
                                    {unread > 99 ? "99+" : unread}
                                </span>
                            )}
                        </span>
                        <span className="truncate font-mono text-[10px] text-text-muted">
                            {profile.url}
                        </span>
                    </span>
                </button>
                <span className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                    <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label="Rename"
                        title="Rename"
                        onClick={(e) => {
                            e.stopPropagation();
                            setRenameValue(profile.name);
                            setRenameOpen(true);
                        }}
                    >
                        <Pencil className="size-3" />
                    </Button>
                    {loggedIn && (
                        <Button
                            type="button"
                            size="icon-sm"
                            variant="ghost"
                            aria-label="Sign out"
                            title="Sign out"
                            onClick={(e) => {
                                e.stopPropagation();
                                forgetServer(profile.id);
                            }}
                        >
                            <LogOut className="size-3" />
                        </Button>
                    )}
                    <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label="Remove"
                        title="Remove"
                        onClick={(e) => {
                            e.stopPropagation();
                            setRemoveOpen(true);
                        }}
                    >
                        <Trash2 className="size-3 text-danger" />
                    </Button>
                </span>
                {shortcut && (
                    <span className="font-mono text-[10px] text-text-muted">
                        {shortcut}
                    </span>
                )}
            </div>

            <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
                <DialogContent className="sm:max-w-[400px]">
                    <DialogHeader>
                        <DialogTitle>Rename server</DialogTitle>
                        <DialogDescription>
                            The display name shows up in the switcher and the
                            Manage Servers dialog.
                        </DialogDescription>
                    </DialogHeader>
                    <Input
                        autoFocus
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") commitRename();
                            if (e.key === "Escape") setRenameOpen(false);
                        }}
                    />
                    <DialogFooter>
                        <Button variant="outline" onClick={() => setRenameOpen(false)}>
                            Cancel
                        </Button>
                        <Button onClick={commitRename} disabled={!renameValue.trim()}>
                            Save
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <AlertDialog open={removeOpen} onOpenChange={setRemoveOpen}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Remove {profile.name}?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The saved URL, display name, and refresh token will
                            be deleted from this client. The server itself is
                            untouched — you can always add it back.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                forgetAndRemoveServer(profile.id);
                                onCloseMenu();
                            }}
                        >
                            Remove
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}

function dotColorForState(state: ConnectionState, loggedIn: boolean): string {
    if (!loggedIn) return palette.textMuted;
    switch (state) {
        case "connected":
            return palette.info;
        case "connecting":
            return palette.warning;
        case "errored":
            return palette.danger;
        case "closed":
        case "idle":
        default:
            return palette.textMuted;
    }
}

// --- Hooks (mirror of the originals from ServerRail.tsx) -------------

let serverListVersion = 0;
onServersChange(() => {
    serverListVersion++;
});

function useServerList(): ServerProfile[] {
    const v = useSyncExternalStore(
        (fn) => onServersChange(fn),
        () => serverListVersion,
        () => serverListVersion,
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

function useActiveServer(): ServerProfile | null {
    // Derive the active profile from the already-stable list +
    // active-id hooks rather than subscribing again. A naïve
    // `useSyncExternalStore(..., getActiveServer, ...)` infinite-loops
    // because `getActiveServer()` parses localStorage and returns a
    // fresh object reference on every call — React thinks the
    // snapshot changed and re-renders forever (Minified React error
    // #185 → "Maximum update depth exceeded"). The list snapshot is
    // already keyed on `serverListVersion` and the id snapshot is a
    // primitive string, so this derivation is stable across renders.
    const id = useActiveServerId();
    const profiles = useServerList();
    return useMemo(
        () => (id ? profiles.find((p) => p.id === id) ?? null : null),
        [id, profiles],
    );
}

function useConnectionState(serverId: string): ConnectionState {
    const [state, setState] = useState(() => connState(serverId));
    useEffect(() => {
        return onConnectionStateChange((id, s) => {
            if (id === serverId) setState(s);
        });
    }, [serverId]);
    return state;
}

let sessionVersion = 0;
onSessionChange(() => {
    sessionVersion++;
});

function useHasPersistedSession(serverId: string): boolean {
    useSyncExternalStore(
        (fn) => onSessionChange(fn),
        () => sessionVersion,
        () => sessionVersion,
    );
    return useMemo(
        () => hasPersistedSession(serverId),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [serverId, sessionVersion],
    );
}
