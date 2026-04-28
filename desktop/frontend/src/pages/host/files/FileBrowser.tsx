import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from "react";
import { DndContext, PointerSensor, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import type { SortingState } from "@tanstack/react-table";
import {
    ChevronUp,
    Eye,
    LayoutGrid,
    LayoutList,
    Loader2,
    Map as MapIcon,
    PanelRightOpen,
    Pencil,
    Rows2,
    Rows3,
    SlidersHorizontal,
    X,
} from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../../../lib/humanizeError";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import RefreshButton from "../../../components/RefreshButton";
import FileContextMenu from "./FileContextMenu";

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
import { basename, humanize } from "../../../lib/format";

import FileTable from "./FileTable";
import FileGrid from "./FileGrid";
import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "@/components/ui/resizable";
import { useViewMode } from "./useViewMode";
import {
    ArchiveFormat,
    archiveExtension,
    suggestedArchiveFilename,
} from "./archive";
import FolderArchiveDialog from "./FolderArchiveDialog";
import { quickPathsForHost } from "./quickPaths";
import type { Host } from "../../../lib/api";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
import { useDensity } from "./useDensity";
import { useDirectory } from "./useDirectory";
import { useDragDrop } from "./useDragDrop";
import { usePreviewPane } from "./usePreviewPane";

// Lazy-load the editors so CodeMirror's ~300 KiB payload doesn't land
// unless the user actually opens a file to view or edit.
const FileEditor = lazy(() => import("./FileEditor"));
const FileViewerPaged = lazy(() => import("./FileViewerPaged"));
const ImageViewer = lazy(() => import("./ImageViewer"));
const PdfViewer = lazy(() => import("./PdfViewer"));
const MediaViewer = lazy(() => import("./MediaViewer"));
const MarkdownViewer = lazy(() => import("./MarkdownViewer"));
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

// PreviewPaneShell renders the file preview / editor surface for the
// expanded right-hand panel. Extracted from FileBrowser so the
// expanded vs collapsed branches of the layout don't both have to
// inline ~120 lines of viewer dispatch. Mounting state stays the
// same — Suspense boundaries and lazy imports are unchanged from the
// inlined version.
function PreviewPaneShell({
    entry,
    canToggleEdit,
    previewKind,
    editMode,
    setEditMode,
    onClose,
    projectID,
    sessionHash,
    dirPath,
    onDownload,
    onReload,
}: {
    entry: FileEntryDTO | null;
    canToggleEdit: boolean;
    previewKind: ReturnType<typeof pickViewerKind> | null;
    editMode: boolean;
    setEditMode: (fn: (v: boolean) => boolean) => void;
    onClose: () => void;
    projectID: string;
    sessionHash: string;
    dirPath: string;
    onDownload: () => void;
    onReload: () => void;
}) {
    return (
        <div
            className="absolute inset-0 flex flex-col overflow-hidden rounded-md border"
            data-testid="preview-pane"
        >
            <div className="flex items-center justify-between gap-2 border-b px-3 py-1.5 text-sm">
                <span className="truncate font-mono">
                    {entry?.name ?? "Preview"}
                </span>
                <div className="flex items-center gap-1">
                    {/* Edit / View toggle — only meaningful for kinds that
                        have a distinct rendered view (today: markdown). For
                        "text" the editor is the only viewer, so the toggle
                        button stays hidden to avoid suggesting a
                        non-existent "view" mode. */}
                    {canToggleEdit && previewKind === "markdown" && (
                        <Button
                            type="button"
                            size="icon-sm"
                            variant="ghost"
                            aria-label={editMode ? "View rendered" : "Edit source"}
                            title={editMode ? "View rendered" : "Edit source"}
                            aria-pressed={editMode}
                            onClick={() => setEditMode((v) => !v)}
                        >
                            {editMode ? (
                                <Eye className="size-3.5" />
                            ) : (
                                <Pencil className="size-3.5" />
                            )}
                        </Button>
                    )}
                    <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label="Close preview"
                        onClick={onClose}
                    >
                        <X className="size-3.5" />
                    </Button>
                </div>
            </div>
            <div className="flex-1 overflow-hidden">
                {entry ? (
                    <Suspense
                        fallback={
                            <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                                <Loader2 className="size-4 animate-spin" />
                                Loading editor…
                            </div>
                        }
                    >
                        {(() => {
                            const fullPath = joinPath(dirPath, entry.name);
                            const kind = pickViewerKind(entry.mime, entry.name);
                            if (kind === "image") {
                                return (
                                    <ImageViewer
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={fullPath}
                                        size={entry.size}
                                        mime={entry.mime}
                                    />
                                );
                            }
                            if (kind === "pdf") {
                                return (
                                    <PdfViewer
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={fullPath}
                                        size={entry.size}
                                    />
                                );
                            }
                            if (kind === "video" || kind === "audio") {
                                return (
                                    <MediaViewer
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={fullPath}
                                        size={entry.size}
                                        kind={kind}
                                        mime={entry.mime}
                                    />
                                );
                            }
                            if (
                                kind === "markdown" &&
                                entry.size <= SMALL_FILE_LIMIT &&
                                !editMode
                            ) {
                                return (
                                    <MarkdownViewer
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={fullPath}
                                        size={entry.size}
                                    />
                                );
                            }
                            if (entry.size > SMALL_FILE_LIMIT) {
                                return (
                                    <FileViewerPaged
                                        projectID={projectID}
                                        sessionHash={sessionHash}
                                        path={fullPath}
                                        size={entry.size}
                                        onDownload={onDownload}
                                    />
                                );
                            }
                            return (
                                <FileEditor
                                    projectID={projectID}
                                    sessionHash={sessionHash}
                                    path={fullPath}
                                    size={entry.size}
                                    onSaved={onReload}
                                />
                            );
                        })()}
                    </Suspense>
                ) : (
                    <div className="flex h-full items-center justify-center px-4 text-center text-sm text-muted-foreground">
                        Select a single file to preview.
                    </div>
                )}
            </div>
        </div>
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
    // Preview pane state: open/closed + persisted width. The pane's
    // contents are derived from the current selection (single-file
    // selection → viewer; otherwise placeholder, but the pane stays
    // open so toggling between rows feels stable).
    const preview = usePreviewPane();
    const [showNewFolder, setShowNewFolder] = useState(false);
    const [showNewFile, setShowNewFile] = useState(false);
    const [showRename, setShowRename] = useState(false);
    const [showChmod, setShowChmod] = useState(false);
    const [showDelete, setShowDelete] = useState(false);
    const [showArchive, setShowArchive] = useState(false);
    const [viewMode, setViewMode] = useViewMode();
    const [density, setDensity] = useDensity();
    // editMode forces the CodeMirror editor over read-only renderers
    // (today: MarkdownViewer). It resets whenever the previewed entry
    // changes so opening a different file always lands on the default
    // viewer for that file's kind.
    const [editMode, setEditMode] = useState(false);

    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    );

    // Reset selection whenever we navigate — avoids stale selections
    // carrying across directories. Preview-open / width state is
    // intentionally preserved: the user's layout preference shouldn't
    // collapse on every cd.
    useEffect(() => {
        setSelected(new Set());
    }, [dir.path]);

    const selectedEntries = useMemo(
        () => dir.entries.filter((e) => selected.has(e.name)),
        [dir.entries, selected],
    );

    // The previewable entry — null when the pane is closed, the
    // selection is empty / multi, or the single selection is a
    // directory or symlink. The pane itself still renders when
    // preview.open is true; previewEntry==null just means "show
    // placeholder, not viewer".
    const previewEntry = useMemo<FileEntryDTO | null>(() => {
        if (!preview.open) return null;
        if (selectedEntries.length !== 1) return null;
        const e = selectedEntries[0];
        if (e.isDir || e.isSymlink) return null;
        return e;
    }, [preview.open, selectedEntries]);

    // Reset editMode whenever the previewed entry changes — switching
    // files should always land on the file's default viewer rather
    // than carrying the previous file's "Edit" state forward.
    useEffect(() => {
        setEditMode(false);
    }, [previewEntry?.name]);

    // isEditableEntry — whether the row's right-click menu should
    // surface an "Edit" item. Today: text + markdown files under the
    // CodeMirror full-load threshold. Larger files route to the read-
    // only paged viewer so the editor doesn't apply.
    const isEditableEntry = useCallback((entry: FileEntryDTO): boolean => {
        if (entry.isDir || entry.isSymlink) return false;
        if (entry.size > SMALL_FILE_LIMIT) return false;
        const k = pickViewerKind(entry.mime, entry.name);
        return k === "text" || k === "markdown";
    }, []);

    // editorKind tells the preview-pane dispatch whether to switch
    // away from the default renderer. Today MarkdownViewer is the only
    // renderer with a distinct read-only mode; "text" already mounts
    // the editor by default.
    const previewKind = useMemo(() => {
        if (!previewEntry) return null;
        return pickViewerKind(previewEntry.mime, previewEntry.name);
    }, [previewEntry]);
    const canToggleEdit = !!(
        previewEntry &&
        previewEntry.size <= SMALL_FILE_LIMIT &&
        (previewKind === "markdown" || previewKind === "text")
    );

    // Right-click on a row: build a FileContextMenu wired to the same
    // toolbar handlers. Selection is reconciled on open so a
    // right-click against an unselected row first selects it (matching
    // OS conventions); right-click inside an existing multi-selection
    // keeps the whole set as the action target.
    const wrapRowWithContextMenu = useCallback(
        (entry: FileEntryDTO, node: React.ReactNode): React.ReactNode => {
            const isInSelection = selected.has(entry.name);
            const targets =
                isInSelection && selectedEntries.length > 0 ? selectedEntries : [entry];
            const fullPath = joinPath(dir.path, entry.name);
            const editable = targets.length === 1 && isEditableEntry(entry);
            return (
                <FileContextMenu
                    variant={{ kind: "row", entries: targets }}
                    onOpenChange={(open) => {
                        if (open && !isInSelection) {
                            setSelected(new Set([entry.name]));
                        }
                    }}
                    onOpen={() => openEntry(entry)}
                    onEdit={
                        editable
                            ? () => {
                                  setSelected(new Set([entry.name]));
                                  preview.setOpen(true);
                                  setEditMode(true);
                              }
                            : undefined
                    }
                    onDownload={handleDownloadClick}
                    onRename={
                        targets.length === 1 ? () => setShowRename(true) : undefined
                    }
                    onChmod={
                        targets.length === 1 ? () => setShowChmod(true) : undefined
                    }
                    onCopyPath={async () => {
                        try {
                            await navigator.clipboard.writeText(fullPath);
                            toast.success("Copied path");
                        } catch (err) {
                            toast.error(`copy: ${humanizeError(err)}`);
                        }
                    }}
                    onCopyName={async () => {
                        try {
                            await navigator.clipboard.writeText(entry.name);
                            toast.success("Copied name");
                        } catch (err) {
                            toast.error(`copy: ${humanizeError(err)}`);
                        }
                    }}
                    onDelete={() => setShowDelete(true)}
                >
                    {node}
                </FileContextMenu>
            );
        },
        // selectedEntries depends on selected; including both keeps the
        // closure's view of the selection set fresh on each render.
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [dir.path, selected, selectedEntries.length, isEditableEntry],
    );

    // --- DnD: OS drop + container-level droppable for "drop into this
    // directory" (the breadcrumb also registers a droppable per crumb).
    const { dragOver, dropHandlers } = useDragDrop({
        projectID,
        sessionHash,
        currentPath: dir.path,
        // Mirror handleUploadClick: pop the drawer open the moment a
        // drop sequence begins so the operator immediately sees
        // progress regardless of which entry point they used.
        onStart: () => setTransfersDrawerOpen(true),
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
        // File: select + open the preview pane. Selection drives which
        // viewer renders, so the same path handles both
        // double-click and Enter from the keyboard handler below.
        setSelected(new Set([entry.name]));
        preview.setOpen(true);
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
        setTransfersDrawerOpen(true);
        try {
            const remotePaths = selectedEntries.map((e) => joinPath(dir.path, e.name));
            await DownloadArchive(projectID, sessionHash, remotePaths, dst, format);
            toast.success(
                `Downloaded ${names.length} item${names.length === 1 ? "" : "s"} as ${archiveExtension(format).slice(1)}`,
            );
        } catch (err) {
            toast.error(`archive: ${humanizeError(err)}`);
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
            if (shouldSkipBrowserShortcut(ev.target, preview.open)) return;
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
            } else if (ev.key === " " && selectedEntries.length === 1) {
                // Quick-Look style toggle: Space on a single selection
                // opens the preview pane; pressing again closes it.
                // The native browser Space-scroll is suppressed so
                // toggling doesn't also page-down the file list.
                ev.preventDefault();
                preview.toggle();
            } else if (ev.key === "Escape" && preview.open) {
                ev.preventDefault();
                preview.close();
            } else if (ev.key === "Enter" && selectedEntries.length === 1) {
                ev.preventDefault();
                openEntry(selectedEntries[0]);
            }
        }
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [goUp, selectedEntries.map((e) => e.name).join("|"), dir.path, preview.open]);

    const crumbs = splitCrumbs(dir.path);
    // Quick-jump destinations are now rendered as a `Go to ▾` dropdown
    // instead of a chip row, so we read the data layer directly here.
    // quickPathsForHost is pure (see ./quickPaths.ts) and returns null
    // while the host fetch is still in flight.
    const quickPaths = quickPathsForHost(host);
    const statusText = `${dir.entries.length} item${dir.entries.length === 1 ? "" : "s"}${
        selectedEntries.length > 0
            ? ` · ${selectedEntries.length} selected · ${humanize(
                  selectedEntries.reduce((acc, e) => acc + (e.size || 0), 0),
              )}`
            : ""
    }${!dir.eof ? ` · first ${dir.entries.length} of ${dir.total}` : ""}`;
    // The Preview panel collapses to a 24 px right-edge rail whenever
    // there's no concrete previewable file in scope — i.e. preview was
    // explicitly closed, or the selection is empty / multi / non-file.
    // The rail keeps the affordance visible without sacrificing 38 % of
    // the viewport to a placeholder. Click rail → opens preview.
    const previewExpanded = preview.open && !!previewEntry;

    return (
        <DndContext sensors={sensors}>
            <div className="flex h-full min-h-0 flex-col gap-1.5">
                {/* Single chrome row: left side carries ↑ + ⟳ + breadcrumb;
                    right side carries the listing counter, a `Go to ▾`
                    dropdown (platform-aware quick paths), and a `View`
                    popover (density + layout). Every other action
                    (New file / folder, Upload, Download, Rename, Chmod,
                    Delete) lives in the right-click context menu. Refresh
                    stays as a one-click icon because re-fetching is the
                    highest-frequency action. */}
                <div
                    data-testid="files-chrome"
                    className="flex items-center gap-2"
                >
                    <div
                        data-testid="files-breadcrumb-row"
                        className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
                    >
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
                        <RefreshButton
                            variant="ghost"
                            iconOnly
                            loading={dir.loading}
                            onClick={dir.reload}
                            aria-label="Refresh"
                            title="Refresh"
                            data-testid="files-refresh"
                        />
                        {crumbs.map((c, idx) => {
                            // splitCrumbs always emits the root segment
                            // first with label "/". Rendering an extra
                            // "/" separator before the next crumb gave
                            // operators a stuttering "//home/..." path —
                            // suppress the separator when the previous
                            // crumb is already the root slash.
                            const showSep = idx > 0 && crumbs[idx - 1].label !== "/";
                            return (
                                <div key={c.path} className="flex items-center gap-1">
                                    {showSep && (
                                        <span className="text-muted-foreground">/</span>
                                    )}
                                    <CrumbDroppable
                                        path={c.path}
                                        label={c.label}
                                        onClick={() => dir.cd(c.path)}
                                        isLast={idx === crumbs.length - 1}
                                    />
                                </div>
                            );
                        })}
                    </div>
                    {/* Listing counter — operators previously had to glance
                        at a separate bottom strip for "N items, M selected"
                        which broke flow when navigating by click. Inlined
                        here so both the path and the cardinality live in
                        the same eye-line. */}
                    <span
                        data-testid="files-status"
                        className="hidden whitespace-nowrap text-[11px] text-muted-foreground sm:inline"
                    >
                        {statusText}
                    </span>
                    {quickPaths && quickPaths.length > 0 && (
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <Button
                                    type="button"
                                    size="icon-sm"
                                    variant="ghost"
                                    aria-label="Go to common path"
                                    title="Go to common path"
                                    data-testid="files-goto"
                                >
                                    <MapIcon className="size-3.5" />
                                </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="min-w-[180px]">
                                <DropdownMenuLabel>Go to</DropdownMenuLabel>
                                <DropdownMenuSeparator />
                                {quickPaths.map((p) => (
                                    <DropdownMenuItem
                                        key={p.path}
                                        onSelect={() => dir.cd(p.path)}
                                        title={p.title}
                                        className="font-mono text-xs"
                                    >
                                        {p.label}
                                        <span className="ml-auto text-[10px] text-muted-foreground">
                                            {p.path}
                                        </span>
                                    </DropdownMenuItem>
                                ))}
                            </DropdownMenuContent>
                        </DropdownMenu>
                    )}
                    <Popover>
                        <PopoverTrigger asChild>
                            <Button
                                type="button"
                                size="icon-sm"
                                variant="ghost"
                                aria-label="View options"
                                title="View options"
                                data-testid="files-view"
                            >
                                <SlidersHorizontal className="size-3.5" />
                            </Button>
                        </PopoverTrigger>
                        <PopoverContent
                            align="end"
                            className="w-auto min-w-[200px] p-3"
                        >
                            <div className="flex flex-col gap-3 text-xs">
                                <div className="flex flex-col gap-1.5">
                                    <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                                        Density
                                    </span>
                                    <div className="flex items-center rounded-md border">
                                        <Button
                                            type="button"
                                            size="icon-sm"
                                            variant={density === "compact" ? "secondary" : "ghost"}
                                            aria-label="Compact density"
                                            aria-pressed={density === "compact"}
                                            onClick={() => setDensity("compact")}
                                            title="Compact rows"
                                        >
                                            <Rows3 className="size-3.5" />
                                        </Button>
                                        <Button
                                            type="button"
                                            size="icon-sm"
                                            variant={density === "comfortable" ? "secondary" : "ghost"}
                                            aria-label="Comfortable density"
                                            aria-pressed={density === "comfortable"}
                                            onClick={() => setDensity("comfortable")}
                                            title="Comfortable rows"
                                        >
                                            <Rows2 className="size-3.5" />
                                        </Button>
                                    </div>
                                </div>
                                <div className="flex flex-col gap-1.5">
                                    <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                                        Layout
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
                        </PopoverContent>
                    </Popover>
                </div>

                {/* Browser + preview split. The preview pane is "either /
                    or": when a single previewable file is selected and the
                    pane is open, it renders as a resizable panel (panel
                    group autoSaveId persists the width). Otherwise it
                    collapses to a 24 px right-edge rail so the listing
                    reclaims the full width — no more 38 % wasted on a
                    "Select a single file to preview" placeholder. The two
                    branches don't share the same ResizablePanelGroup
                    because the rail must sit *outside* the group; mounting
                    it inside as a fixed-width sibling fights the
                    percent-based layout the panel group uses. */}
                {previewExpanded ? (
                    <ResizablePanelGroup
                        direction="horizontal"
                        autoSaveId="files-preview-split"
                        className="min-h-0 flex-1"
                    >
                        <ResizablePanel id="files-list" defaultSize={62} minSize={30} className="relative">
                            <FileContextMenu
                                variant={{ kind: "empty" }}
                                onNewFile={() => setShowNewFile(true)}
                                onNewFolder={() => setShowNewFolder(true)}
                                onUploadHere={handleUploadClick}
                                onRefresh={dir.reload}
                            >
                                <div
                                    className={cn(
                                        "absolute inset-0 overflow-auto rounded-md border",
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
                                            wrapRow={wrapRowWithContextMenu}
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
                                            wrapRow={wrapRowWithContextMenu}
                                            density={density}
                                        />
                                    )}
                                </div>
                            </FileContextMenu>
                        </ResizablePanel>
                        <ResizableHandle className="mx-1 bg-transparent" />
                        <ResizablePanel
                            id="files-preview"
                            defaultSize={38}
                            minSize={20}
                            maxSize={70}
                            className="relative"
                        >
                            <PreviewPaneShell
                                entry={previewEntry}
                                canToggleEdit={canToggleEdit}
                                previewKind={previewKind}
                                editMode={editMode}
                                setEditMode={setEditMode}
                                onClose={preview.close}
                                projectID={projectID}
                                sessionHash={sessionHash}
                                dirPath={dir.path}
                                onDownload={handleDownloadClick}
                                onReload={dir.reload}
                            />
                        </ResizablePanel>
                    </ResizablePanelGroup>
                ) : (
                    <div className="flex min-h-0 flex-1 gap-1">
                        <FileContextMenu
                            variant={{ kind: "empty" }}
                            onNewFile={() => setShowNewFile(true)}
                            onNewFolder={() => setShowNewFolder(true)}
                            onUploadHere={handleUploadClick}
                            onRefresh={dir.reload}
                        >
                            <div
                                className={cn(
                                    "h-full flex-1 overflow-auto rounded-md border",
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
                                        wrapRow={wrapRowWithContextMenu}
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
                                        wrapRow={wrapRowWithContextMenu}
                                        density={density}
                                    />
                                )}
                            </div>
                        </FileContextMenu>
                        {/* Right-edge rail: a 24 px column that keeps the
                            "open preview" affordance visible at all times.
                            Click expands to the full panel; if the
                            current selection isn't a previewable file,
                            the panel mounts on the placeholder. The
                            tooltip explains the behaviour without
                            stealing space. */}
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <button
                                    type="button"
                                    aria-label="Open preview"
                                    data-testid="files-preview-rail"
                                    onClick={() => preview.setOpen(true)}
                                    className="flex w-6 shrink-0 items-center justify-center rounded-md border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                                >
                                    <PanelRightOpen className="size-3.5" />
                                </button>
                            </TooltipTrigger>
                            <TooltipContent side="left">Preview</TooltipContent>
                        </Tooltip>
                    </div>
                )}
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
