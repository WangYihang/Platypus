import * as React from "react";
import { Fragment } from "react";
import { useDraggable, useDroppable, useDndMonitor } from "@dnd-kit/core";

import { cn } from "@/lib/cn";
import type { FileEntryDTO } from "../../../platform/App.web";
import { humanize } from "../../../lib/format";
import Thumbnail from "./Thumbnail";
import { isHiddenEntry, pickFileIcon } from "./fileIcons";

interface Props {
    entries: FileEntryDTO[];
    currentPath: string;
    selectedNames: Set<string>;
    setSelectedNames: (names: Set<string>) => void;
    onOpen: (entry: FileEntryDTO) => void;
    // Same internal-move surface as FileTable so the toggle is a pure
    // presentation switch — selection / drag semantics match.
    onInternalMove?: (fromEntry: FileEntryDTO, toDir: string) => void;
    // Same row-wrapper escape hatch FileTable exposes — used by
    // FileBrowser to wrap each tile in a right-click context menu.
    wrapRow?: (entry: FileEntryDTO, node: React.ReactNode) => React.ReactNode;
}

// MAX_THUMB_BYTES caps the size of files we will fetch to render a
// preview. Anything larger falls back to the generic file icon so the
// grid doesn't issue huge reads on rare large images. 2 MiB is a
// pragmatic ceiling — large enough for typical screenshots / icons,
// small enough to be cheap when scrolled past.
const MAX_THUMB_BYTES = 2 * 1024 * 1024;

function isImageEntry(e: FileEntryDTO): boolean {
    if (e.isDir || e.isSymlink) return false;
    if (e.mime?.startsWith("image/")) return true;
    return false;
}

interface TileProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "onSelect"> {
    entry: FileEntryDTO;
    isSelected: boolean;
    onSelectChange: (e: React.MouseEvent) => void;
    onOpen: () => void;
    isDroppable: boolean;
    projectID?: string;
    sessionHash?: string;
    fullPath?: string;
}

// `forwardRef` + spread of `...rest` is load-bearing: the per-row
// FileContextMenu wraps each Tile via `<ContextMenuTrigger asChild>`,
// which clones the child to attach an `onContextMenu` listener and a
// ref. Without forwardRef + props passthrough Radix has no way to
// reach the underlying <button>, the inner trigger silently does
// nothing, and right-clicks bubble up to the outer empty-area
// trigger — so the user sees the New file / Upload here menu instead
// of the row's Open / Download / Rename / Delete menu.
const Tile = React.forwardRef<HTMLButtonElement, TileProps>(function Tile(
    {
        entry,
        isSelected,
        onSelectChange,
        onOpen,
        isDroppable,
        projectID,
        sessionHash,
        fullPath,
        onClick: extraOnClick,
        onDoubleClick: extraOnDoubleClick,
        ...rest
    },
    forwardedRef,
) {
    const drag = useDraggable({
        id: `grid-row:${entry.name}`,
        data: { entry },
    });
    const drop = useDroppable({
        id: `grid-drop:${entry.name}`,
        data: { dirName: entry.name, isDir: entry.isDir },
        disabled: !isDroppable,
    });

    const setRefs = (node: HTMLButtonElement | null) => {
        drag.setNodeRef(node);
        if (isDroppable) drop.setNodeRef(node);
        if (typeof forwardedRef === "function") forwardedRef(node);
        else if (forwardedRef) forwardedRef.current = node;
    };

    const showThumb =
        isImageEntry(entry) &&
        entry.size > 0 &&
        entry.size <= MAX_THUMB_BYTES &&
        projectID &&
        sessionHash &&
        fullPath;

    const { Icon, color } = pickFileIcon(entry);
    const dim = isHiddenEntry(entry);

    return (
        <button
            ref={setRefs}
            type="button"
            aria-label={entry.name}
            data-selected={isSelected || undefined}
            className={cn(
                // 2026-04 density pass: tile shrunk from w-28 / size-16 /
                // size-10 (icon) → w-20 / size-12 / size-8 so the grid
                // renders ~12 columns on a 1440 viewport instead of ~7.
                // Padding and gap tightened to match. The file size
                // line is already 10 px so it stays unchanged.
                "group relative flex w-20 flex-col items-center gap-0.5 rounded-md border border-transparent p-1.5 text-center text-[11px]",
                "hover:bg-accent",
                isSelected && "border-primary bg-accent",
                drop.isOver && isDroppable && "outline outline-2 outline-primary",
                drag.isDragging && "opacity-60",
                dim && !isSelected && "opacity-60",
            )}
            onClick={(e) => {
                onSelectChange(e);
                extraOnClick?.(e);
            }}
            onDoubleClick={(e) => {
                onOpen();
                extraOnDoubleClick?.(e);
            }}
            {...drag.listeners}
            {...drag.attributes}
            {...rest}
        >
            <div className="flex size-12 items-center justify-center overflow-hidden rounded bg-muted">
                {showThumb ? (
                    <Thumbnail
                        projectID={projectID!}
                        sessionHash={sessionHash!}
                        path={fullPath!}
                        mime={entry.mime}
                    />
                ) : (
                    <Icon className={cn("size-8", color)} />
                )}
            </div>
            <div className="w-full truncate font-mono">{entry.name}</div>
            {!entry.isDir && (
                <div className="text-[10px] text-muted-foreground">{humanize(entry.size)}</div>
            )}
        </button>
    );
});

export default function FileGrid({
    entries,
    selectedNames,
    setSelectedNames,
    onOpen,
    onInternalMove,
    currentPath,
    projectID,
    sessionHash,
    wrapRow,
}: Props & { projectID?: string; sessionHash?: string }) {
    useDndMonitor({
        onDragEnd(event) {
            if (!onInternalMove) return;
            const from = event.active.data.current?.entry as FileEntryDTO | undefined;
            const toDir = event.over?.data.current?.dirName as string | undefined;
            const toIsDir = event.over?.data.current?.isDir as boolean | undefined;
            if (!from || !toDir || !toIsDir) return;
            if (from.name === toDir) return;
            onInternalMove(from, toDir);
        },
    });

    function toggleRow(entry: FileEntryDTO, ev: React.MouseEvent) {
        const next = new Set(selectedNames);
        if (ev.shiftKey || ev.metaKey || ev.ctrlKey) {
            if (next.has(entry.name)) next.delete(entry.name);
            else next.add(entry.name);
        } else {
            next.clear();
            next.add(entry.name);
        }
        setSelectedNames(next);
    }

    if (entries.length === 0) {
        return (
            <div className="py-10 text-center text-sm text-muted-foreground">
                Empty directory
            </div>
        );
    }

    return (
        <div className="flex flex-wrap content-start gap-1.5 p-1.5">
            {entries.map((e) => {
                const tile = (
                    <Tile
                        key={e.name}
                        entry={e}
                        isSelected={selectedNames.has(e.name)}
                        isDroppable={e.isDir}
                        onSelectChange={(ev) => toggleRow(e, ev)}
                        onOpen={() => onOpen(e)}
                        projectID={projectID}
                        sessionHash={sessionHash}
                        fullPath={
                            projectID && sessionHash
                                ? `${currentPath.replace(/\/$/, "")}/${e.name}`
                                : undefined
                        }
                    />
                );
                return wrapRow ? (
                    <Fragment key={e.name}>{wrapRow(e, tile)}</Fragment>
                ) : (
                    tile
                );
            })}
        </div>
    );
}
