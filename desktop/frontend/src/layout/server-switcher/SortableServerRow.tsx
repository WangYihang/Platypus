import { CSSProperties, useState } from "react";
import { GripVertical, LogOut, Pencil, Trash2 } from "lucide-react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS as DndCSS } from "@dnd-kit/utilities";

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
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";

import { palette, radius } from "../theme";
import {
    ServerProfile,
    avatarFor,
    renameServer,
} from "../../lib/servers";
import { ConnectionState } from "../../lib/notify";
import { forgetAndRemoveServer, forgetServer } from "../../lib/auth";
import { useUnread } from "../../lib/unread";

import { useConnectionState, useHasPersistedSession } from "./hooks";

interface Props {
    profile: ServerProfile;
    index: number;
    active: boolean;
    onActivate: () => void;
    onCloseMenu: () => void;
}

// One row inside the ServerSwitcher dropdown. Includes its own
// rename / remove dialogs because both are scoped to the row.
export default function SortableServerRow({
    profile,
    index,
    active,
    onActivate,
    onCloseMenu,
}: Props) {
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
