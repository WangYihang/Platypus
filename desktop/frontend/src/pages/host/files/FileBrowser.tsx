import { useCallback, useEffect, useMemo, useState } from "react";
import { DndContext } from "@dnd-kit/core";
import type { SortingState } from "@tanstack/react-table";
import { PanelRightOpen } from "lucide-react";
import { toast } from "sonner";

import { humanizeError } from "../../../lib/humanizeError";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useDragSensors } from "../../../lib/dnd";
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
import Split from "@/components/ui/Split";
import { useViewMode } from "./useViewMode";
import { isHiddenEntry } from "./fileIcons";
import { sortEntries } from "./sortEntries";
import { usePreference } from "../../../lib/preferences";
import {
    ArchiveFormat,
    archiveExtension,
    suggestedArchiveFilename,
} from "./archive";
import FolderArchiveDialog from "./FolderArchiveDialog";
import { quickPathsForHost } from "./quickPaths";
import type { Host } from "../../../lib/api";
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

import FilePreview, { SMALL_FILE_LIMIT } from "./FilePreview";
import FilesChrome from "./FilesChrome";
import { pickViewerKind } from "./viewerKind";

interface Props {
    projectID: string;
    sessionHash: string;
    host?: Host | null;
}

export default function FileBrowser({ projectID, sessionHash, host = null }: Props) {
    const dir = useDirectory(projectID, sessionHash);
    const { setOpen: setTransfersDrawerOpen } = useTransfersDrawer();
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }]);
    const preview = usePreviewPane();
    const [showNewFolder, setShowNewFolder] = useState(false);
    const [showNewFile, setShowNewFile] = useState(false);
    const [showRename, setShowRename] = useState(false);
    const [showChmod, setShowChmod] = useState(false);
    const [showDelete, setShowDelete] = useState(false);
    const [showArchive, setShowArchive] = useState(false);
    const [viewMode, setViewMode] = useViewMode();
    const [density, setDensity] = useDensity();
    const [showHidden, setShowHidden] = usePreference("ui.files.showHidden");
    const [editMode, setEditMode] = useState(false);

    // Apply hidden-file filtering + the active sort once at the
    // browser level so both views (FileTable / FileGrid) render the
    // same ordering. FileTable used to drive its own getSortedRowModel;
    // moving sorting up here lets the toolbar's sort menu reorder the
    // grid view too, and lets us add sort ids ("type") that don't map
    // to a column accessor.
    const visibleEntries = useMemo(() => {
        const filtered = showHidden
            ? dir.entries
            : dir.entries.filter((e) => !isHiddenEntry(e));
        return sortEntries(filtered, sorting);
    }, [dir.entries, showHidden, sorting]);

    const hiddenCount = useMemo(
        () => dir.entries.reduce((n, e) => n + (isHiddenEntry(e) ? 1 : 0), 0),
        [dir.entries],
    );

    const sensors = useDragSensors(5);

    // Reset selection on cd; preview-open / width state intentionally persists.
    useEffect(() => {
        setSelected(new Set());
    }, [dir.path]);

    const selectedEntries = useMemo(
        () => visibleEntries.filter((e) => selected.has(e.name)),
        [visibleEntries, selected],
    );

    const previewEntry = useMemo<FileEntryDTO | null>(() => {
        if (!preview.open) return null;
        if (selectedEntries.length !== 1) return null;
        const e = selectedEntries[0];
        if (e.isDir || e.isSymlink) return null;
        return e;
    }, [preview.open, selectedEntries]);

    // Reset editMode whenever the previewed entry changes.
    useEffect(() => {
        setEditMode(false);
    }, [previewEntry?.name]);

    const isEditableEntry = useCallback((entry: FileEntryDTO): boolean => {
        if (entry.isDir || entry.isSymlink) return false;
        if (entry.size > SMALL_FILE_LIMIT) return false;
        const k = pickViewerKind(entry.mime, entry.name);
        return k === "text" || k === "markdown";
    }, []);

    const previewKind = useMemo(() => {
        if (!previewEntry) return null;
        return pickViewerKind(previewEntry.mime, previewEntry.name);
    }, [previewEntry]);
    const canToggleEdit = !!(
        previewEntry &&
        previewEntry.size <= SMALL_FILE_LIMIT &&
        (previewKind === "markdown" || previewKind === "text")
    );

    // Right-click on a row reconciles selection (right-click on an unselected row
    // first selects it; right-click within an existing multi-selection keeps the set).
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
        // selectedEntries depends on selected; including both keeps the closure fresh.
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [dir.path, selected, selectedEntries.length, isEditableEntry],
    );

    const { dragOver, dropHandlers } = useDragDrop({
        projectID,
        sessionHash,
        currentPath: dir.path,
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
            toast.message(`${entry.name} → ${entry.symlinkTarget || "(unreadable)"}`);
            return;
        }
        setSelected(new Set([entry.name]));
        preview.setOpen(true);
    }

    async function handleUploadClick() {
        const src = await PickFileToUpload("Choose local file");
        if (!src) return;
        const name = basename(src);
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
        // Folders / multi-selections package as a single archive built server-side
        // in chunks; the dialog confirms the format.
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
        const sourcePath = joinPath(dir.path, from.name);
        const sibling = dir.entries.find((e) => e.name === toDirName && e.isDir);
        const destDir = sibling
            ? joinPath(dir.path, toDirName)
            : toDirName.startsWith("/")
              ? toDirName
              : joinPath(dir.path, toDirName);
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

    // Container-level keyboard shortcuts. Skip when CodeMirror is mounted (Backspace
    // there must edit text, not pop the directory) or when typing in any input.
    useEffect(() => {
        function onKey(ev: KeyboardEvent) {
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
                // Quick-Look style toggle: Space opens/closes the preview pane.
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
    const quickPaths = quickPathsForHost(host);
    const visibleCount = visibleEntries.length;
    const statusText = `${visibleCount} item${visibleCount === 1 ? "" : "s"}${
        !showHidden && hiddenCount > 0 ? ` · ${hiddenCount} hidden` : ""
    }${
        selectedEntries.length > 0
            ? ` · ${selectedEntries.length} selected · ${humanize(
                  selectedEntries.reduce((acc, e) => acc + (e.size || 0), 0),
              )}`
            : ""
    }${!dir.eof ? ` · first ${dir.entries.length} of ${dir.total}` : ""}`;
    const previewExpanded = preview.open && !!previewEntry;

    const listing = (
        <FileContextMenu
            variant={{ kind: "empty" }}
            onNewFile={() => setShowNewFile(true)}
            onNewFolder={() => setShowNewFolder(true)}
            onUploadHere={handleUploadClick}
            onRefresh={dir.reload}
        >
            <div
                className={cn(
                    "h-full w-full overflow-auto rounded-md border",
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
                        entries={visibleEntries}
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
                        entries={visibleEntries}
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
    );

    return (
        <DndContext sensors={sensors}>
            <div className="flex h-full min-h-0 flex-col gap-1.5">
                <FilesChrome
                    crumbs={crumbs}
                    canGoUp={dir.path !== "/"}
                    onGoUp={goUp}
                    onCd={dir.cd}
                    onRefresh={dir.reload}
                    refreshLoading={dir.loading}
                    statusText={statusText}
                    quickPaths={quickPaths}
                    viewMode={viewMode}
                    setViewMode={setViewMode}
                    density={density}
                    setDensity={setDensity}
                    showHidden={showHidden}
                    setShowHidden={setShowHidden}
                    sorting={sorting}
                    setSorting={setSorting}
                />

                {previewExpanded ? (
                    <Split
                        direction="row"
                        storageKey="files-preview-split"
                        defaultPercent={62}
                        minPercent={30}
                        maxPercent={80}
                        className="flex-1"
                    >
                        {listing}
                        <FilePreview
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
                    </Split>
                ) : (
                    <div className="flex min-h-0 flex-1 gap-1">
                        {listing}
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
