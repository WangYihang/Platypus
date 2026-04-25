import {
    CSSProperties,
    MouseEvent,
    useCallback,
    useEffect,
    useMemo,
    useState,
    useSyncExternalStore,
} from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Plus, Settings } from "lucide-react";
import {
    DndContext,
    DragEndEvent,
    PointerSensor,
    useSensor,
    useSensors,
} from "@dnd-kit/core";
import {
    SortableContext,
    arrayMove,
    useSortable,
    verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS as DndCSS } from "@dnd-kit/utilities";

import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "@/components/ui/tooltip";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
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

import { palette, radius, space } from "./theme";
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

const RAIL_WIDTH = 64;
const TILE_SIZE = 40;

interface Props {
    onAddServer: () => void;
    onManageServers: () => void;
}

// ServerRail is the Slack-style left rail. One tile per saved
// ServerProfile; click to switch, right-click for rename / sign out
// / remove, drag to reorder. `+` opens AddServerDialog, gear opens
// ManageServersDialog. Mounted once in ShellChrome so it's visible
// on every authenticated route.
export default function ServerRail({ onAddServer, onManageServers }: Props) {
    const profiles = useServerList();
    const activeId = useActiveServerId();
    const navigate = useNavigate();
    const location = useLocation();

    const sensors = useSensors(
        useSensor(PointerSensor, {
            // 6px activation threshold so a plain click still fires
            // onClick instead of starting a drag.
            activationConstraint: { distance: 6 },
        }),
    );

    const handleDragEnd = (e: DragEndEvent) => {
        const { active, over } = e;
        if (!over || active.id === over.id) return;
        const ids = profiles.map((p) => p.id);
        const from = ids.indexOf(String(active.id));
        const to = ids.indexOf(String(over.id));
        if (from < 0 || to < 0) return;
        reorderServers(arrayMove(ids, from, to));
    };

    const onTileClick = useCallback(
        async (profile: ServerProfile) => {
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

    return (
        <aside
            data-testid="server-rail"
            style={{
                width: RAIL_WIDTH,
                flexShrink: 0,
                height: "100%",
                background: palette.rail,
                borderRight: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                padding: `${space[3]}px 0`,
                gap: space[2],
            }}
        >
            <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
                <SortableContext
                    items={profiles.map((p) => p.id)}
                    strategy={verticalListSortingStrategy}
                >
                    <div
                        style={{
                            display: "flex",
                            flexDirection: "column",
                            alignItems: "center",
                            gap: space[2],
                            flex: 1,
                            width: "100%",
                            overflow: "auto",
                            paddingBottom: space[2],
                        }}
                    >
                        {profiles.map((p, i) => (
                            <ServerTile
                                key={p.id}
                                profile={p}
                                index={i}
                                active={p.id === activeId}
                                onActivate={() => void onTileClick(p)}
                            />
                        ))}
                    </div>
                </SortableContext>
            </DndContext>

            <div
                style={{
                    width: 32,
                    height: 1,
                    background: palette.border,
                }}
            />

            <RailButton
                ariaLabel="Add server"
                tooltip="Add server"
                onClick={onAddServer}
                testid="server-rail-add"
            >
                <Plus className="size-4" />
            </RailButton>
            <RailButton
                ariaLabel="Manage servers"
                tooltip="Manage servers"
                onClick={onManageServers}
                testid="server-rail-manage"
            >
                <Settings className="size-4" />
            </RailButton>
        </aside>
    );
}

interface TileProps {
    profile: ServerProfile;
    index: number;
    active: boolean;
    onActivate: () => void;
}

function ServerTile({ profile, index, active, onActivate }: TileProps) {
    const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
        useSortable({ id: profile.id });
    const avatar = avatarFor(profile);
    const state = useConnectionState(profile.id);
    const unread = useUnread(profile.id);
    const loggedIn = useHasPersistedSession(profile.id);

    const dotColor = dotColorForState(state, loggedIn);

    const tileStyle: CSSProperties = {
        transform: DndCSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.6 : 1,
    };

    const shortcut =
        index < 9
            ? (typeof navigator !== "undefined" && /Mac/i.test(navigator.platform)
                  ? "⌘"
                  : "Ctrl") +
              (index + 1)
            : null;

    return (
        <Tooltip>
            <ContextMenuWrapper profile={profile}>
                <TooltipTrigger asChild>
                    <button
                        ref={setNodeRef}
                        data-testid={`server-tile-${index}`}
                        data-active={active}
                        aria-label={`Switch to ${profile.name}`}
                        onClick={onActivate}
                        {...attributes}
                        {...listeners}
                        style={{
                            position: "relative",
                            width: TILE_SIZE,
                            height: TILE_SIZE,
                            borderRadius: radius.lg,
                            background: avatar.bg,
                            color: avatar.fg,
                            fontFamily: "var(--font-geist-sans)",
                            fontWeight: 600,
                            fontSize: 16,
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            cursor: "pointer",
                            border: "none",
                            outline: active
                                ? `2px solid ${palette.textPrimary}`
                                : "2px solid transparent",
                            outlineOffset: 2,
                            transition: "outline-color 120ms ease",
                            ...tileStyle,
                        }}
                    >
                        {avatar.letter}

                        {active && (
                            <span
                                aria-hidden
                                data-testid="rail-active-indicator"
                                style={{
                                    position: "absolute",
                                    left: -12,
                                    top: "50%",
                                    transform: "translateY(-50%)",
                                    width: 3,
                                    height: TILE_SIZE - 10,
                                    borderRadius: 2,
                                    background: palette.textPrimary,
                                }}
                            />
                        )}

                        <span
                            aria-hidden
                            style={{
                                position: "absolute",
                                right: -2,
                                bottom: -2,
                                width: 10,
                                height: 10,
                                borderRadius: 999,
                                background: dotColor,
                                border: `2px solid ${palette.rail}`,
                            }}
                        />

                        {unread > 0 && !active && (
                            <span
                                aria-label={`${unread} unread`}
                                style={{
                                    position: "absolute",
                                    top: -4,
                                    right: -4,
                                    minWidth: 18,
                                    height: 18,
                                    padding: "0 4px",
                                    borderRadius: 999,
                                    background: palette.danger,
                                    color: "#fff",
                                    fontSize: 10,
                                    fontWeight: 600,
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                    border: `2px solid ${palette.rail}`,
                                }}
                            >
                                {unread > 99 ? "99+" : unread}
                            </span>
                        )}
                    </button>
                </TooltipTrigger>
            </ContextMenuWrapper>
            <TooltipContent side="right" sideOffset={12}>
                <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                    <span style={{ fontWeight: 600 }}>{profile.name}</span>
                    <span
                        style={{
                            fontFamily: "var(--font-geist-mono)",
                            color: palette.textSecondary,
                            fontSize: 11,
                        }}
                    >
                        {profile.url}
                    </span>
                    {shortcut && (
                        <span style={{ color: palette.textMuted, fontSize: 11 }}>
                            {shortcut} to switch
                        </span>
                    )}
                </div>
            </TooltipContent>
        </Tooltip>
    );
}

function ContextMenuWrapper({
    profile,
    children,
}: {
    profile: ServerProfile;
    children: React.ReactNode;
}) {
    const [menuOpen, setMenuOpen] = useState(false);
    const [renameOpen, setRenameOpen] = useState(false);
    const [renameValue, setRenameValue] = useState(profile.name);
    const [removeOpen, setRemoveOpen] = useState(false);

    const onContextMenu = (e: MouseEvent) => {
        e.preventDefault();
        setMenuOpen(true);
    };

    const commitRename = () => {
        const next = renameValue.trim();
        if (next && next !== profile.name) {
            renameServer(profile.id, next);
        }
        setRenameOpen(false);
    };

    return (
        <>
            <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                <DropdownMenuTrigger asChild>
                    <span onContextMenu={onContextMenu} style={{ display: "inline-flex" }}>
                        {children}
                    </span>
                </DropdownMenuTrigger>
                <DropdownMenuContent side="right" align="start">
                    <DropdownMenuItem
                        onSelect={() => {
                            setRenameValue(profile.name);
                            setRenameOpen(true);
                        }}
                    >
                        Rename
                    </DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => forgetServer(profile.id)}>
                        Sign out
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                        variant="destructive"
                        onSelect={() => setRemoveOpen(true)}
                    >
                        Remove
                    </DropdownMenuItem>
                </DropdownMenuContent>
            </DropdownMenu>

            <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
                <DialogContent className="sm:max-w-[400px]">
                    <DialogHeader>
                        <DialogTitle>Rename server</DialogTitle>
                        <DialogDescription>
                            The display name shows up on the rail tile and in
                            the Manage Servers dialog.
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
                            onClick={() => forgetAndRemoveServer(profile.id)}
                        >
                            Remove
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}

interface RailButtonProps {
    ariaLabel: string;
    tooltip: string;
    onClick: () => void;
    children: React.ReactNode;
    testid?: string;
}

function RailButton({ ariaLabel, tooltip, onClick, children, testid }: RailButtonProps) {
    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <button
                    data-testid={testid}
                    aria-label={ariaLabel}
                    onClick={onClick}
                    style={{
                        width: TILE_SIZE,
                        height: TILE_SIZE,
                        borderRadius: radius.lg,
                        background: palette.surface,
                        color: palette.textSecondary,
                        border: `1px solid ${palette.border}`,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        cursor: "pointer",
                        transition: "color 120ms ease, background 120ms ease",
                    }}
                >
                    {children}
                </button>
            </TooltipTrigger>
            <TooltipContent side="right" sideOffset={12}>
                {tooltip}
            </TooltipContent>
        </Tooltip>
    );
}

function dotColorForState(state: ConnectionState, loggedIn: boolean): string {
    if (!loggedIn) return palette.textMuted;
    switch (state) {
        case "connected":
            return palette.successDot;
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

// --- Hooks ----------------------------------------------------------

// listServers() returns a fresh array on each call. useSyncExternalStore
// treats reference changes as "updated" and would re-render forever.
// Track a monotonic version instead and rehydrate the list in a useMemo
// keyed on it.
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
