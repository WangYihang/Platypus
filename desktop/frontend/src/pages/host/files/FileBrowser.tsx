import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from "react";
import { DndContext, PointerSensor, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import type { SortingState } from "@tanstack/react-table";
import {
    ChevronUp,
    Download,
    Edit,
    FolderPlus,
    Loader2,
    Lock,
    RefreshCw,
    Trash2,
    Upload,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";

import {
    Chmod,
    DeleteFile,
    DownloadFile,
    Mkdir,
    PickFileToUpload,
    PickSaveLocation,
    RenameFile,
    UploadFile,
} from "../../../../wailsjs/go/app/App";
import type { FileEntryDTO } from "../../../platform/App.web";
import { basename } from "../../../lib/format";

import FileTable from "./FileTable";
import { ChmodDialog, DeleteConfirmDialog, NewFolderDialog, RenameDialog } from "./dialogs";
import { joinPath, parentPath, splitCrumbs } from "./paths";
import { useDirectory } from "./useDirectory";
import { useDragDrop } from "./useDragDrop";

// Lazy-load the editors so CodeMirror's ~300 KiB payload doesn't land
// unless the user actually opens a file to view or edit.
const FileEditor = lazy(() => import("./FileEditor"));
const FileViewerPaged = lazy(() => import("./FileViewerPaged"));

// Files larger than this open in the read-only paged viewer. Anything
// below is loaded whole into CodeMirror. 5 MiB is empirically the
// breakpoint where full-load editing stays responsive.
const SMALL_FILE_LIMIT = 5 * 1024 * 1024;

interface Props {
    projectID: string;
    sessionHash: string;
}

// CrumbDroppable is a breadcrumb segment that accepts internal drops —
// drop a file onto "/etc" to move it up one directory, etc.
function CrumbDroppable({
    path,
    label,
    onClick,
    isLast,
}: {
    path: string;
    label: string;
    onClick: () => void;
    isLast: boolean;
}) {
    const { setNodeRef, isOver } = useDroppable({
        id: `crumb:${path}`,
        data: { dirName: label, isDir: true, fullPath: path, isCrumb: true },
    });
    return (
        <button
            ref={setNodeRef}
            type="button"
            onClick={onClick}
            className={cn(
                "font-mono text-sm hover:underline",
                isLast ? "text-foreground" : "text-muted-foreground",
                isOver && "rounded bg-accent px-1",
            )}
        >
            {label}
        </button>
    );
}

export default function FileBrowser({ projectID, sessionHash }: Props) {
    const dir = useDirectory(projectID, sessionHash);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }]);
    const [openingEntry, setOpeningEntry] = useState<FileEntryDTO | null>(null);
    const [showNewFolder, setShowNewFolder] = useState(false);
    const [showRename, setShowRename] = useState(false);
    const [showChmod, setShowChmod] = useState(false);
    const [showDelete, setShowDelete] = useState(false);

    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    );

    // Reset selection whenever we navigate — avoids stale selections
    // carrying across directories.
    useEffect(() => {
        setSelected(new Set());
        setOpeningEntry(null);
    }, [dir.path]);

    const selectedEntries = useMemo(
        () => dir.entries.filter((e) => selected.has(e.name)),
        [dir.entries, selected],
    );

    // --- DnD: OS drop + container-level droppable for "drop into this
    // directory" (the breadcrumb also registers a droppable per crumb).
    const { dragOver, dropHandlers } = useDragDrop({
        projectID,
        sessionHash,
        currentPath: dir.path,
        onFinished: () => {
            dir.reload();
            toast.success("Upload finished");
        },
        onError: (e) => toast.error(e),
        onProgress: (p) => {
            if (p.filename && p.total > 1) {
                toast.message(`Uploading ${p.filename} (${p.done + 1}/${p.total})`);
            }
        },
    });

    const goUp = useCallback(() => {
        if (dir.path === "/") return;
        dir.cd(parentPath(dir.path));
    }, [dir]);

    async function openEntry(entry: FileEntryDTO) {
        if (entry.isDir) {
            dir.cd(joinPath(dir.path, entry.name));
            return;
        }
        if (entry.isSymlink) {
            // Surface symlink targets but don't follow automatically.
            toast.message(`${entry.name} → ${entry.symlinkTarget || "(unreadable)"}`);
            return;
        }
        setOpeningEntry(entry);
    }

    async function handleUploadClick() {
        const src = await PickFileToUpload("Choose local file");
        if (!src) return;
        const name = basename(src);
        try {
            await UploadFile(projectID, sessionHash, joinPath(dir.path, name), src);
            toast.success(`Uploaded ${name}`);
            dir.reload();
        } catch (err) {
            toast.error(`upload: ${String(err instanceof Error ? err.message : err)}`);
        }
    }

    async function handleDownloadClick() {
        if (selectedEntries.length !== 1 || selectedEntries[0].isDir) {
            toast.error("Select a single file to download");
            return;
        }
        const entry = selectedEntries[0];
        const dst = await PickSaveLocation("Save to", entry.name);
        if (!dst) return;
        try {
            await DownloadFile(projectID, sessionHash, joinPath(dir.path, entry.name), dst);
            toast.success(`Downloaded ${entry.name}`);
        } catch (err) {
            toast.error(`download: ${String(err instanceof Error ? err.message : err)}`);
        }
    }

    async function handleCreateFolder(name: string) {
        try {
            await Mkdir(projectID, sessionHash, joinPath(dir.path, name), false, 0o755);
            toast.success(`Created ${name}`);
            dir.reload();
        } catch (err) {
            toast.error(`mkdir: ${String(err instanceof Error ? err.message : err)}`);
        }
    }

    async function handleRename(newName: string) {
        const entry = selectedEntries[0];
        if (!entry) return;
        try {
            await RenameFile(
                projectID,
                sessionHash,
                joinPath(dir.path, entry.name),
                joinPath(dir.path, newName),
            );
            toast.success(`Renamed to ${newName}`);
            dir.reload();
        } catch (err) {
            toast.error(`rename: ${String(err instanceof Error ? err.message : err)}`);
        }
    }

    async function handleChmod(mode: number) {
        const entry = selectedEntries[0];
        if (!entry) return;
        try {
            await Chmod(projectID, sessionHash, joinPath(dir.path, entry.name), mode);
            toast.success(`chmod ${mode.toString(8)} ${entry.name}`);
            dir.reload();
        } catch (err) {
            toast.error(`chmod: ${String(err instanceof Error ? err.message : err)}`);
        }
    }

    async function handleDelete() {
        for (const entry of selectedEntries) {
            try {
                await DeleteFile(projectID, sessionHash, joinPath(dir.path, entry.name), entry.isDir);
            } catch (err) {
                toast.error(`delete ${entry.name}: ${String(err instanceof Error ? err.message : err)}`);
                dir.reload();
                return;
            }
        }
        toast.success(`Deleted ${selectedEntries.length} item(s)`);
        dir.reload();
    }

    function handleInternalMove(from: FileEntryDTO, toDirName: string) {
        // toDirName is a sibling directory in the current listing — we
        // only register droppables on dirs + breadcrumb segments, so
        // the destination is always a resolvable directory path.
        const sourcePath = joinPath(dir.path, from.name);
        // Breadcrumb droppables stash the full path; sibling dirs don't.
        // Look in the current entries for a matching dir; fall back to
        // treating toDirName as a full path.
        const sibling = dir.entries.find((e) => e.name === toDirName && e.isDir);
        const destDir = sibling ? joinPath(dir.path, toDirName) : toDirName.startsWith("/") ? toDirName : joinPath(dir.path, toDirName);
        const destPath = joinPath(destDir, from.name);
        if (sourcePath === destPath) return;
        (async () => {
            try {
                await RenameFile(projectID, sessionHash, sourcePath, destPath);
                toast.success(`Moved ${from.name} → ${destDir}`);
                dir.reload();
            } catch (err) {
                toast.error(`move: ${String(err instanceof Error ? err.message : err)}`);
            }
        })();
    }

    // --- Keyboard shortcuts on the whole browser. We register at the
    // container level so arrow keys etc. don't need the table focused.
    useEffect(() => {
        function onKey(ev: KeyboardEvent) {
            if ((ev.target as HTMLElement)?.matches?.("input, textarea")) return;
            if (ev.key === "Backspace") {
                ev.preventDefault();
                goUp();
            } else if (ev.key === "F2" && selectedEntries.length === 1) {
                ev.preventDefault();
                setShowRename(true);
            } else if (ev.key === "Delete" && selectedEntries.length > 0) {
                ev.preventDefault();
                setShowDelete(true);
            } else if ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === "n") {
                ev.preventDefault();
                setShowNewFolder(true);
            } else if (ev.key === "Enter" && selectedEntries.length === 1) {
                ev.preventDefault();
                openEntry(selectedEntries[0]);
            }
        }
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [goUp, selectedEntries.map((e) => e.name).join("|"), dir.path]);

    const crumbs = splitCrumbs(dir.path);

    return (
        <DndContext sensors={sensors}>
            <div className="flex h-full min-h-[520px] flex-col gap-3">
                {/* Breadcrumb */}
                <div className="flex items-center gap-1 overflow-x-auto">
                    <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={goUp}
                        disabled={dir.path === "/"}
                        title="Up"
                    >
                        <ChevronUp className="size-3.5" />
                    </Button>
                    {crumbs.map((c, idx) => (
                        <div key={c.path} className="flex items-center gap-1">
                            {idx > 0 && <span className="text-muted-foreground">/</span>}
                            <CrumbDroppable
                                path={c.path}
                                label={c.label}
                                onClick={() => dir.cd(c.path)}
                                isLast={idx === crumbs.length - 1}
                            />
                        </div>
                    ))}
                </div>

                {/* Toolbar */}
                <div
                    data-testid="files-toolbar"
                    className="flex flex-wrap items-center gap-2"
                >
                    <Button type="button" variant="outline" size="sm" onClick={dir.reload} disabled={dir.loading}>
                        {dir.loading ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <RefreshCw className="size-3.5" />
                        )}
                        Refresh
                    </Button>
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setShowNewFolder(true)}
                    >
                        <FolderPlus className="size-3.5" />
                        New folder
                    </Button>
                    <Button type="button" variant="outline" size="sm" onClick={handleUploadClick}>
                        <Upload className="size-3.5" />
                        Upload
                    </Button>
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={handleDownloadClick}
                        disabled={selectedEntries.length !== 1 || selectedEntries[0]?.isDir}
                        title="Drag-out to OS not supported; use this button instead."
                    >
                        <Download className="size-3.5" />
                        Download
                    </Button>
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setShowRename(true)}
                        disabled={selectedEntries.length !== 1}
                    >
                        <Edit className="size-3.5" />
                        Rename
                    </Button>
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setShowChmod(true)}
                        disabled={selectedEntries.length !== 1}
                    >
                        <Lock className="size-3.5" />
                        Chmod
                    </Button>
                    <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        onClick={() => setShowDelete(true)}
                        disabled={selectedEntries.length === 0}
                    >
                        <Trash2 className="size-3.5" />
                        Delete
                    </Button>
                    <div className="ml-auto text-xs text-muted-foreground">
                        {selectedEntries.length > 0 && `${selectedEntries.length} selected · `}
                        {dir.entries.length} entries
                        {!dir.eof && ` (showing first ${dir.entries.length} of ${dir.total})`}
                    </div>
                </div>

                {/* Browser + editor split */}
                <div className="flex flex-1 gap-3 overflow-hidden">
                    <div
                        className={cn(
                            "flex-1 overflow-auto rounded-md border",
                            dragOver && "bg-accent/40 outline outline-2 outline-primary",
                        )}
                        {...dropHandlers}
                    >
                        {dir.error ? (
                            <div className="p-6 text-sm text-red-500">
                                Load error: {dir.error}
                            </div>
                        ) : (
                            <FileTable
                                entries={dir.entries}
                                currentPath={dir.path}
                                selectedNames={selected}
                                setSelectedNames={setSelected}
                                onOpen={openEntry}
                                sorting={sorting}
                                setSorting={setSorting}
                                onInternalMove={handleInternalMove}
                            />
                        )}
                    </div>
                    {openingEntry && (
                        <div className="flex-1 overflow-hidden rounded-md border">
                            <Suspense
                                fallback={
                                    <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                                        <Loader2 className="size-4 animate-spin" />
                                        Loading editor…
                                    </div>
                                }
                            >
                                {openingEntry.size > SMALL_FILE_LIMIT ? (
                                    <FileViewerPaged
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={joinPath(dir.path, openingEntry.name)}
                                        size={openingEntry.size}
                                        onDownload={handleDownloadClick}
                                    />
                                ) : (
                                    <FileEditor
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={joinPath(dir.path, openingEntry.name)}
                                        size={openingEntry.size}
                                        onSaved={dir.reload}
                                    />
                                )}
                            </Suspense>
                        </div>
                    )}
                </div>
            </div>

            <NewFolderDialog
                open={showNewFolder}
                onOpenChange={setShowNewFolder}
                parentPath={dir.path}
                onConfirm={handleCreateFolder}
            />
            <RenameDialog
                open={showRename}
                onOpenChange={setShowRename}
                entry={selectedEntries[0] ?? null}
                onConfirm={handleRename}
            />
            <ChmodDialog
                open={showChmod}
                onOpenChange={setShowChmod}
                entry={selectedEntries[0] ?? null}
                onConfirm={handleChmod}
            />
            <DeleteConfirmDialog
                open={showDelete}
                onOpenChange={setShowDelete}
                entries={selectedEntries}
                onConfirm={handleDelete}
            />
        </DndContext>
    );
}
