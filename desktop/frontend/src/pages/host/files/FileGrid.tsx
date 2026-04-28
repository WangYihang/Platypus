import { Fragment } from "react";
import { useDraggable, useDroppable, useDndMonitor } from "@dnd-kit/core";
import { File, FileSymlink, Folder } from "lucide-react";

import { cn } from "@/lib/cn";
import type { FileEntryDTO } from "../../../platform/App.web";
import { humanize } from "../../../lib/format";
import Thumbnail from "./Thumbnail";

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

function Tile({
    entry,
    isSelected,
    onSelectChange,
    onOpen,
    isDroppable,
    projectID,
    sessionHash,
    fullPath,
}: {
    entry: FileEntryDTO;
    isSelected: boolean;
    onSelectChange: (e: React.MouseEvent) => void;
    onOpen: () => void;
    isDroppable: boolean;
    projectID?: string;
    sessionHash?: string;
    fullPath?: string;
}) {
    const drag = useDraggable({
        id: `grid-row:${entry.name}`,
        data: { entry },
    });
    const drop = useDroppable({
        id: `grid-drop:${entry.name}`,
        data: { dirName: entry.name, isDir: entry.isDir },
        disabled: !isDroppable,
    });

    const setRefs = (node: HTMLElement | null) => {
        drag.setNodeRef(node);
        if (isDroppable) drop.setNodeRef(node);
    };

    const showThumb =
        isImageEntry(entry) &&
        entry.size > 0 &&
        entry.size <= MAX_THUMB_BYTES &&
        projectID &&
        sessionHash &&
        fullPath;

    return (
        <button
            ref={setRefs}
            type="button"
            aria-label={entry.name}
            data-selected={isSelected || undefined}
            className={cn(
                "group relative flex w-28 flex-col items-center gap-1 rounded-md border border-transparent p-2 text-center text-xs",
                "hover:bg-accent",
                isSelected && "border-primary bg-accent",
                drop.isOver && isDroppable && "outline outline-2 outline-primary",
                drag.isDragging && "opacity-60",
            )}
            onClick={onSelectChange}
            onDoubleClick={onOpen}
            {...drag.listeners}
            {...drag.attributes}
        >
            <div className="flex size-16 items-center justify-center overflow-hidden rounded bg-muted">
                {entry.isDir ? (
                    <Folder className="size-10 text-amber-500" />
                ) : entry.isSymlink ? (
                    <FileSymlink className="size-10 text-sky-500" />
                ) : showThumb ? (
                    <Thumbnail
                        projectID={projectID!}
                        sessionHash={sessionHash!}
                        path={fullPath!}
                        mime={entry.mime}
                    />
                ) : (
                    <File className="size-10 text-muted-foreground" />
                )}
            </div>
            <div className="w-full truncate font-mono">{entry.name}</div>
            {!entry.isDir && (
                <div className="text-[10px] text-muted-foreground">{humanize(entry.size)}</div>
            )}
        </button>
    );
}

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
        <div className="flex flex-wrap content-start gap-2 p-2">
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
