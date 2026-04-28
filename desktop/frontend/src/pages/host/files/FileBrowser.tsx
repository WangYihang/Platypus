import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from "react";
import { DndContext, PointerSensor, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import type { SortingState } from "@tanstack/react-table";
import {
    ChevronUp,
    Download,
    Edit,
    FilePlus,
    FolderDown,
    FolderPlus,
    LayoutGrid,
    LayoutList,
    Loader2,
    Lock,
    RefreshCw,
    Trash2,
    Upload,
} from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../../../lib/humanizeError";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";

import {
    Chmod,
    DeleteFile,
    DownloadArchive,
    DownloadFile,
    Mkdir,
    PickFileToUpload,
    PickSaveLocation,
    RenameFile,
    UploadFile,
    WriteFile,
} from "@wails/go/app/App";
import type { FileEntryDTO } from "@wails/go/app/App";
import { basename } from "../../../lib/format";

import FileTable from "./FileTable";
import FileGrid from "./FileGrid";
import { useViewMode } from "./useViewMode";
import {
    ArchiveFormat,
    archiveExtension,
    suggestedArchiveFilename,
} from "./archive";
import FolderArchiveDialog from "./FolderArchiveDialog";
import QuickPaths from "./QuickPaths";
import type { Host } from "../../../lib/api";
import { useTransfersDrawer } from "../../../components/TransfersPill";
import {
    ChmodDialog,
    DeleteConfirmDialog,
    NewFileDialog,
    NewFolderDialog,
    RenameDialog,
} from "./dialogs";
import { joinPath, parentPath, splitCrumbs } from "./paths";
import { shouldSkipBrowserShortcut } from "./keymap";
import { useDirectory } from "./useDirectory";
import { useDragDrop } from "./useDragDrop";

// Lazy-load the editors so CodeMirror's ~300 KiB payload doesn't land
// unless the user actually opens a file to view or edit.
const FileEditor = lazy(() => import("./FileEditor"));
const FileViewerPaged = lazy(() => import("./FileViewerPaged"));
const ImageViewer = lazy(() => import("./ImageViewer"));
const PdfViewer = lazy(() => import("./PdfViewer"));
import { pickViewerKind } from "./viewerKind";

// Files larger than this open in the read-only paged viewer. Anything
// below is loaded whole into CodeMirror. 5 MiB is empirically the
// breakpoint where full-load editing stays responsive.
const SMALL_FILE_LIMIT = 5 * 1024 * 1024;

interface Props {
    projectID: string;
    sessionHash: string;
    // The agent-reported host descriptor. Forwarded down to the
    // QuickPaths chip row so it can pick platform-appropriate
    // roots (Linux: /, ~, /etc, …; Windows: C:\, …). Null while
    // HostView is still loading the host fetch.
    host?: Host | null;
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

export default function FileBrowser({ projectID, sessionHash, host = null }: Props) {
    const dir = useDirectory(projectID, sessionHash);
    // Surface progress immediately when the user kicks off a transfer.
    // The drawer provider lives at the shell root so the hook is
    // always reachable from any host page.
    const { setOpen: setTransfersDrawerOpen } = useTransfersDrawer();
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }]);
    const [openingEntry, setOpeningEntry] = useState<FileEntryDTO | null>(null);
    const [showNewFolder, setShowNewFolder] = useState(false);
    const [showNewFile, setShowNewFile] = useState(false);
    const [showRename, setShowRename] = useState(false);
    const [showChmod, setShowChmod] = useState(false);
    const [showDelete, setShowDelete] = useState(false);
    const [bulkDownloading, setBulkDownloading] = useState(false);
    const [showArchive, setShowArchive] = useState(false);
    const [viewMode, setViewMode] = useViewMode();

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
        // Pop the transfers drawer so the operator can watch progress
        // tick. Doing this before await keeps the UI responsive even
        // on a slow upstream.
        setTransfersDrawerOpen(true);
        try {
            await UploadFile(projectID, sessionHash, joinPath(dir.path, name), src);
            toast.success(`Uploaded ${name}`);
            dir.reload();
        } catch (err) {
            toast.error(`upload: ${humanizeError(err)}`);
        }
    }

    async function handleDownloadClick() {
        if (selectedEntries.length === 0) {
            toast.error("Select at least one entry to download");
            return;
        }
        // Single file → single save dialog. The user can choose any
        // filename so this is the most flexible single-file path.
        if (selectedEntries.length === 1 && !selectedEntries[0].isDir) {
            const entry = selectedEntries[0];
            const dst = await PickSaveLocation("Save to", entry.name);
            if (!dst) return;
            setTransfersDrawerOpen(true);
            try {
                await DownloadFile(projectID, sessionHash, joinPath(dir.path, entry.name), dst);
                toast.success(`Downloaded ${entry.name}`);
            } catch (err) {
                toast.error(`download: ${humanizeError(err)}`);
            }
            return;
        }
        // Anything that includes a folder, or any multi-selection,
        // packages into a single archive — folders can't ride the
        // OS save-as dialog as a tree, and forcing operators into
        // many save dialogs / a hidden "save to dir" picker reads
        // worse than a clear "pick the format" prompt. The dialog
        // keeps the format choice visible and lets the operator
        // confirm what's being packaged. The archive itself is
        // built server-side in chunks so a huge tree never holds
        // anything in memory.
        setShowArchive(true);
    }

    async function handleArchiveConfirm(format: ArchiveFormat) {
        const names = selectedEntries.map((e) => e.name);
        const single = names.length === 1 ? names[0] : names;
        const filename = suggestedArchiveFilename(single, format);
        const dst = await PickSaveLocation("Save archive as", filename);
        if (!dst) {
            setShowArchive(false);
            return;
        }
        setShowArchive(false);
        setBulkDownloading(true);
        setTransfersDrawerOpen(true);
        try {
            const remotePaths = selectedEntries.map((e) => joinPath(dir.path, e.name));
            await DownloadArchive(projectID, sessionHash, remotePaths, dst, format);
            toast.success(
                `Downloaded ${names.length} item${names.length === 1 ? "" : "s"} as ${archiveExtension(format).slice(1)}`,
            );
        } catch (err) {
            toast.error(`archive: ${humanizeError(err)}`);
        } finally {
            setBulkDownloading(false);
        }
    }

    async function handleCreateFolder(name: string) {
        try {
            await Mkdir(projectID, sessionHash, joinPath(dir.path, name), false, 0o755);
            toast.success(`Created ${name}`);
            dir.reload();
        } catch (err) {
            toast.error(`mkdir: ${humanizeError(err)}`);
        }
    }

    async function handleCreateFile(name: string) {
        // Reject path separators — names with "/" would silently
        // create the file in a different directory.
        if (name.includes("/") || name.includes("\\")) {
            toast.error("File name cannot contain '/' or '\\'");
            return;
        }
        try {
            await WriteFile(projectID, sessionHash, joinPath(dir.path, name), [], false);
            toast.success(`Created ${name}`);
            dir.reload();
        } catch (err) {
            toast.error(`new file: ${humanizeError(err)}`);
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
            toast.error(`rename: ${humanizeError(err)}`);
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
            toast.error(`chmod: ${humanizeError(err)}`);
        }
    }

    async function handleDelete() {
        for (const entry of selectedEntries) {
            try {
                await DeleteFile(projectID, sessionHash, joinPath(dir.path, entry.name), entry.isDir);
            } catch (err) {
                toast.error(`delete ${entry.name}: ${humanizeError(err)}`);
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
                toast.error(`move: ${humanizeError(err)}`);
            }
        })();
    }

    // --- Keyboard shortcuts on the whole browser. We register at the
    // container level so arrow keys etc. don't need the table focused.
    useEffect(() => {
        function onKey(ev: KeyboardEvent) {
            // Skip every shortcut when the file editor is mounted —
            // CodeMirror renders into a contenteditable, so Backspace
            // and friends would otherwise close the editor and pop
            // the directory back up. Also skip when typing in any
            // input/contenteditable/role=textbox elsewhere on the
            // page (rename, breadcrumb, search …).
            if (shouldSkipBrowserShortcut(ev.target, openingEntry !== null)) return;
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
    }, [goUp, selectedEntries.map((e) => e.name).join("|"), dir.path, openingEntry]);

    const crumbs = splitCrumbs(dir.path);

    return (
        <DndContext sensors={sensors}>
            <div className="flex h-full min-h-[520px] flex-col gap-3">
                {/* Quick-jump chip row, sits above the breadcrumb so
                    teleporting to a common root is one click away. */}
                <QuickPaths host={host} onSelect={dir.cd} />
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
                        onClick={() => setShowNewFile(true)}
                    >
                        <FilePlus className="size-3.5" />
                        New file
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
                        disabled={selectedEntries.length === 0 || bulkDownloading}
                        title={
                            selectedEntries.length === 0
                                ? "Select files or folders to download"
                                : selectedEntries.length === 1 && !selectedEntries[0]?.isDir
                                  ? "Download to a chosen location"
                                  : "Download all selected entries (folders are mirrored recursively)"
                        }
                    >
                        {bulkDownloading ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : selectedEntries.some((e) => e.isDir) ? (
                            <FolderDown className="size-3.5" />
                        ) : (
                            <Download className="size-3.5" />
                        )}
                        Download
                        {selectedEntries.length > 1 && ` (${selectedEntries.length})`}
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
                    <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
                        <span>
                            {selectedEntries.length > 0 && `${selectedEntries.length} selected · `}
                            {dir.entries.length} entries
                            {!dir.eof && ` (showing first ${dir.entries.length} of ${dir.total})`}
                        </span>
                        <div className="flex items-center rounded-md border">
                            <Button
                                type="button"
                                size="icon-sm"
                                variant={viewMode === "list" ? "secondary" : "ghost"}
                                aria-label="List view"
                                aria-pressed={viewMode === "list"}
                                onClick={() => setViewMode("list")}
                            >
                                <LayoutList className="size-3.5" />
                            </Button>
                            <Button
                                type="button"
                                size="icon-sm"
                                variant={viewMode === "grid" ? "secondary" : "ghost"}
                                aria-label="Grid view"
                                aria-pressed={viewMode === "grid"}
                                onClick={() => setViewMode("grid")}
                            >
                                <LayoutGrid className="size-3.5" />
                            </Button>
                        </div>
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
                        ) : viewMode === "grid" ? (
                            <FileGrid
                                entries={dir.entries}
                                currentPath={dir.path}
                                selectedNames={selected}
                                setSelectedNames={setSelected}
                                onOpen={openEntry}
                                onInternalMove={handleInternalMove}
                                projectID={projectID}
                                sessionHash={sessionHash}
                            />
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
                                {(() => {
                                    const fullPath = joinPath(dir.path, openingEntry.name);
                                    const kind = pickViewerKind(openingEntry.mime, openingEntry.name);
                                    if (kind === "image") {
                                        return (
                                            <ImageViewer
                                                projectID={projectID}
                                                sessionHash={sessionHash}
                                                path={fullPath}
                                                size={openingEntry.size}
                                                mime={openingEntry.mime}
                                            />
                                        );
                                    }
                                    if (kind === "pdf") {
                                        return (
                                            <PdfViewer
                                                projectID={projectID}
                                                sessionHash={sessionHash}
                                                path={fullPath}
                                                size={openingEntry.size}
                                            />
                                        );
                                    }
                                    if (openingEntry.size > SMALL_FILE_LIMIT) {
                                        return (
                                            <FileViewerPaged
                                                projectID={projectID}
                                                sessionHash={sessionHash}
                                                path={fullPath}
                                                size={openingEntry.size}
                                                onDownload={handleDownloadClick}
                                            />
                                        );
                                    }
                                    return (
                                        <FileEditor
                                            projectID={projectID}
                                            sessionHash={sessionHash}
                                            path={fullPath}
                                            size={openingEntry.size}
                                            onSaved={dir.reload}
                                        />
                                    );
                                })()}
                            </Suspense>
                        </div>
                    )}
                </div>
            </div>

            <FolderArchiveDialog
                open={showArchive}
                onOpenChange={setShowArchive}
                names={selectedEntries.map((e) => e.name)}
                onConfirm={handleArchiveConfirm}
            />
            <NewFileDialog
                open={showNewFile}
                onOpenChange={setShowNewFile}
                parentPath={dir.path}
                onConfirm={handleCreateFile}
            />
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
