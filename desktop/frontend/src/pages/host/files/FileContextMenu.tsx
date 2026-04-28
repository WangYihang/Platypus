import { ReactNode } from "react";

import {
    ContextMenu,
    ContextMenuContent,
    ContextMenuItem,
    ContextMenuSeparator,
    ContextMenuShortcut,
    ContextMenuTrigger,
} from "@/components/ui/context-menu";
import type { FileEntryDTO } from "@wails/go/app/App";

// FileContextMenu is the right-click overlay for the file pane. Two
// variants share the primitive:
//
//   row    — over a file or folder row (or a multi-selection); hosts
//            Open/Preview, Download, Rename, Chmod, Copy path/name,
//            Delete.
//   empty  — over the blank area of the pane; hosts New file, New
//            folder, Upload here, Paste (placeholder), Refresh.
//
// Selection management lives outside this component: the row's
// onContextMenu wiring is responsible for selecting the row before
// opening the menu (matching OS conventions). The component just
// renders whichever entries you hand it.
//
// Items hide when the corresponding callback isn't wired so callers
// can scope which actions are exposed (e.g. a viewer-tier user without
// delete perms simply doesn't pass onDelete).

type Variant = { kind: "row"; entries: FileEntryDTO[] } | { kind: "empty" };

interface Props {
    variant: Variant;
    children: ReactNode;
    // Forwarded to Radix's ContextMenu root so callers can react to
    // open/close — most commonly to fix selection state when a user
    // right-clicks an unselected row (the row should become the
    // selection target before the menu's actions resolve).
    onOpenChange?: (open: boolean) => void;
    // Row callbacks.
    onOpen?: () => void;
    // onEdit is offered alongside onOpen for files that mount the
    // CodeMirror editor — the toolbar used to be the only way operators
    // could find that path, which left "where's the edit button?" as a
    // recurring complaint. Wiring it is the caller's call: pass undefined
    // for kinds that have no editor (images, video, …) and the item just
    // hides.
    onEdit?: () => void;
    onDownload?: () => void;
    onRename?: () => void;
    onChmod?: () => void;
    onCopyPath?: () => void;
    onCopyName?: () => void;
    onDelete?: () => void;
    // Empty-area callbacks.
    onNewFile?: () => void;
    onNewFolder?: () => void;
    onUploadHere?: () => void;
    onRefresh?: () => void;
}

export default function FileContextMenu({
    variant,
    children,
    onOpenChange,
    onOpen,
    onEdit,
    onDownload,
    onRename,
    onChmod,
    onCopyPath,
    onCopyName,
    onDelete,
    onNewFile,
    onNewFolder,
    onUploadHere,
    onRefresh,
}: Props) {
    return (
        <ContextMenu onOpenChange={onOpenChange}>
            <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
            <ContextMenuContent>
                {variant.kind === "row"
                    ? renderRowItems(variant.entries, {
                          onOpen,
                          onEdit,
                          onDownload,
                          onRename,
                          onChmod,
                          onCopyPath,
                          onCopyName,
                          onDelete,
                      })
                    : renderEmptyItems({
                          onNewFile,
                          onNewFolder,
                          onUploadHere,
                          onRefresh,
                      })}
            </ContextMenuContent>
        </ContextMenu>
    );
}

function renderRowItems(
    entries: FileEntryDTO[],
    cbs: Pick<
        Props,
        | "onOpen"
        | "onEdit"
        | "onDownload"
        | "onRename"
        | "onChmod"
        | "onCopyPath"
        | "onCopyName"
        | "onDelete"
    >,
) {
    const single = entries.length === 1 ? entries[0] : null;

    return (
        <>
            {cbs.onOpen && (
                <ContextMenuItem onSelect={cbs.onOpen}>
                    Open
                    <ContextMenuShortcut>Enter</ContextMenuShortcut>
                </ContextMenuItem>
            )}
            {cbs.onEdit && (
                <ContextMenuItem onSelect={cbs.onEdit}>
                    Edit
                </ContextMenuItem>
            )}
            {cbs.onDownload && (
                <ContextMenuItem onSelect={cbs.onDownload}>Download</ContextMenuItem>
            )}
            {/* Rename + Chmod only meaningful for a single entry. */}
            {single && cbs.onRename && (
                <ContextMenuItem onSelect={cbs.onRename}>
                    Rename
                    <ContextMenuShortcut>F2</ContextMenuShortcut>
                </ContextMenuItem>
            )}
            {single && cbs.onChmod && (
                <ContextMenuItem onSelect={cbs.onChmod}>Chmod</ContextMenuItem>
            )}
            {(cbs.onCopyPath || cbs.onCopyName) && <ContextMenuSeparator />}
            {cbs.onCopyPath && (
                <ContextMenuItem onSelect={cbs.onCopyPath}>Copy path</ContextMenuItem>
            )}
            {cbs.onCopyName && (
                <ContextMenuItem onSelect={cbs.onCopyName}>Copy name</ContextMenuItem>
            )}
            {cbs.onDelete && (
                <>
                    <ContextMenuSeparator />
                    <ContextMenuItem variant="destructive" onSelect={cbs.onDelete}>
                        Delete
                        <ContextMenuShortcut>Del</ContextMenuShortcut>
                    </ContextMenuItem>
                </>
            )}
        </>
    );
}

function renderEmptyItems(
    cbs: Pick<Props, "onNewFile" | "onNewFolder" | "onUploadHere" | "onRefresh">,
) {
    return (
        <>
            {cbs.onNewFile && (
                <ContextMenuItem onSelect={cbs.onNewFile}>
                    New file
                    <ContextMenuShortcut>Ctrl+N</ContextMenuShortcut>
                </ContextMenuItem>
            )}
            {cbs.onNewFolder && (
                <ContextMenuItem onSelect={cbs.onNewFolder}>
                    New folder
                    <ContextMenuShortcut>Ctrl+Shift+N</ContextMenuShortcut>
                </ContextMenuItem>
            )}
            {cbs.onUploadHere && (
                <ContextMenuItem onSelect={cbs.onUploadHere}>Upload here</ContextMenuItem>
            )}
            <ContextMenuSeparator />
            {/* Paste is wired-but-disabled until clipboard work lands;
                seeing it greyed-out is better UX than the menu being
                missing the OS-conventional item. */}
            <ContextMenuItem disabled>Paste</ContextMenuItem>
            {cbs.onRefresh && (
                <>
                    <ContextMenuSeparator />
                    <ContextMenuItem onSelect={cbs.onRefresh}>
                        Refresh
                        <ContextMenuShortcut>F5</ContextMenuShortcut>
                    </ContextMenuItem>
                </>
            )}
        </>
    );
}
