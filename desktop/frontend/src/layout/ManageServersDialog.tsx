import { useMemo, useState, useSyncExternalStore } from "react";
import { LogOut, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

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
    renameServer,
    useServersStore,
} from "../lib/servers";
import {
    forgetAndRemoveServer,
    forgetServer,
    hasPersistedSession,
    onSessionChange,
} from "../lib/auth";

interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onAddServer: () => void;
}

// ManageServersDialog lists every saved profile with rename / sign
// out / remove actions. Opened from the rail's gear button or from
// the command palette.
export default function ManageServersDialog({
    open,
    onOpenChange,
    onAddServer,
}: Props) {
    const servers = useServerSnapshot();
    const [confirmRemove, setConfirmRemove] = useState<ServerProfile | null>(null);

    return (
        <>
            <Dialog open={open} onOpenChange={onOpenChange}>
                <DialogContent className="sm:max-w-[600px]">
                    <DialogHeader>
                        <DialogTitle>Servers</DialogTitle>
                        <DialogDescription>
                            Manage the Platypus servers saved in this client. Signing
                            out keeps the URL saved; removing deletes it entirely.
                        </DialogDescription>
                    </DialogHeader>

                    <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
                        {servers.length === 0 && (
                            <div
                                style={{
                                    padding: space[4],
                                    color: palette.textMuted,
                                    textAlign: "center",
                                    fontSize: 13,
                                }}
                            >
                                No servers saved yet.
                            </div>
                        )}
                        {servers.map((p) => (
                            <ServerRow
                                key={p.id}
                                profile={p}
                                onRemove={() => setConfirmRemove(p)}
                            />
                        ))}
                    </div>

                    <DialogFooter>
                        <Button
                            variant="outline"
                            onClick={() => {
                                onOpenChange(false);
                                onAddServer();
                            }}
                        >
                            <Plus className="size-3.5" />
                            Add server
                        </Button>
                        <Button onClick={() => onOpenChange(false)}>Done</Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <AlertDialog
                open={confirmRemove !== null}
                onOpenChange={(v) => {
                    if (!v) setConfirmRemove(null);
                }}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            Remove {confirmRemove?.name}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            The saved URL, display name, and refresh token will be
                            deleted from this client. The server itself is untouched
                            — you can always add it back.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                if (!confirmRemove) return;
                                forgetAndRemoveServer(confirmRemove.id);
                                toast.success(`Removed ${confirmRemove.name}`);
                                setConfirmRemove(null);
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

function ServerRow({
    profile,
    onRemove,
}: {
    profile: ServerProfile;
    onRemove: () => void;
}) {
    const [editing, setEditing] = useState(false);
    const [name, setName] = useState(profile.name);
    const loggedIn = useHasPersistedSession(profile.id);
    const avatar = avatarFor(profile);

    const commit = () => {
        if (name.trim() && name.trim() !== profile.name) {
            renameServer(profile.id, name.trim());
        } else {
            setName(profile.name);
        }
        setEditing(false);
    };

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[3],
                padding: space[3],
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                background: palette.surface,
            }}
        >
            <div
                aria-hidden
                style={{
                    width: 36,
                    height: 36,
                    borderRadius: radius.md,
                    background: avatar.bg,
                    color: avatar.fg,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    fontWeight: 600,
                    fontSize: 14,
                    flexShrink: 0,
                }}
            >
                {avatar.letter}
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
                {editing ? (
                    <Input
                        autoFocus
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        onBlur={commit}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") commit();
                            if (e.key === "Escape") {
                                setName(profile.name);
                                setEditing(false);
                            }
                        }}
                        className="h-7"
                    />
                ) : (
                    <div
                        onClick={() => setEditing(true)}
                        role="button"
                        style={{
                            fontWeight: 500,
                            color: palette.textPrimary,
                            cursor: "text",
                        }}
                        title="Click to rename"
                    >
                        {profile.name}
                    </div>
                )}
                <div
                    style={{
                        fontFamily: "var(--font-geist-mono)",
                        color: palette.textMuted,
                        fontSize: 11,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {profile.url}
                </div>
            </div>
            <Button
                size="sm"
                variant="outline"
                onClick={() => forgetServer(profile.id)}
                disabled={!loggedIn}
                title={loggedIn ? "Sign out (keeps profile)" : "Already signed out"}
            >
                <LogOut className="size-3.5" />
                Sign out
            </Button>
            <Button size="sm" variant="destructive" onClick={onRemove}>
                <Trash2 className="size-3.5" />
                Remove
            </Button>
        </div>
    );
}

// --- Hooks ----------------------------------------------------------

let sessionVersion = 0;
onSessionChange(() => {
    sessionVersion++;
});

function useServerSnapshot(): ServerProfile[] {
    return useServersStore((s) => s.profiles);
}

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
