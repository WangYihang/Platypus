import { Suspense, lazy } from "react";
import { Eye, Loader2, Pencil, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { FileEntryDTO } from "@wails/go/app/App";

import { joinPath } from "./paths";
import { pickViewerKind } from "./viewerKind";

// Lazy-loaded so CodeMirror's ~300 KiB payload doesn't land unless a file is opened.
const FileEditor = lazy(() => import("./FileEditor"));
const FileViewerPaged = lazy(() => import("./FileViewerPaged"));
const ImageViewer = lazy(() => import("./ImageViewer"));
const PdfViewer = lazy(() => import("./PdfViewer"));
const MediaViewer = lazy(() => import("./MediaViewer"));
const MarkdownViewer = lazy(() => import("./MarkdownViewer"));

// Files larger than 5 MiB use the paged read-only viewer; smaller files load
// whole into CodeMirror.
const SMALL_FILE_LIMIT = 5 * 1024 * 1024;

interface Props {
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
}

export default function FilePreview({
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
}: Props) {
    return (
        <div
            className="flex h-full w-full flex-col overflow-hidden rounded-md border"
            data-testid="preview-pane"
        >
            <div className="flex items-center justify-between gap-2 border-b px-3 py-1.5 text-sm">
                <span className="truncate font-mono">{entry?.name ?? "Preview"}</span>
                <div className="flex items-center gap-1">
                    {/* Edit/View toggle is only shown for markdown — text has no rendered view. */}
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
                        <Viewer
                            entry={entry}
                            projectID={projectID}
                            sessionHash={sessionHash}
                            dirPath={dirPath}
                            editMode={editMode}
                            onDownload={onDownload}
                            onReload={onReload}
                        />
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

function Viewer({
    entry,
    projectID,
    sessionHash,
    dirPath,
    editMode,
    onDownload,
    onReload,
}: {
    entry: FileEntryDTO;
    projectID: string;
    sessionHash: string;
    dirPath: string;
    editMode: boolean;
    onDownload: () => void;
    onReload: () => void;
}) {
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
    if (kind === "markdown" && entry.size <= SMALL_FILE_LIMIT && !editMode) {
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
}

export { SMALL_FILE_LIMIT };
