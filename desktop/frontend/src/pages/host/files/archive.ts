// archive.ts is the single source of truth for "what compression
// formats does the folder-download dialog offer, and how does each
// map onto a file extension / MIME type / display label?". The
// dialog component, the FileBrowser glue code, and the Go backend
// (via `format` query) all read these helpers so they don't drift.
//
// Order in ARCHIVE_FORMATS is the order the dialog renders them.
// `tar.gz` is first because it's the most common Linux operator
// expectation; zip exists for cross-OS / Windows operators.

export type ArchiveFormat = "tar" | "tar.gz" | "zip";

export const ARCHIVE_FORMATS: readonly ArchiveFormat[] = [
    "tar.gz",
    "tar",
    "zip",
] as const;

export function archiveExtension(format: ArchiveFormat): string {
    switch (format) {
        case "tar":
            return ".tar";
        case "tar.gz":
            return ".tar.gz";
        case "zip":
            return ".zip";
    }
}

export function archiveMimeType(format: ArchiveFormat): string {
    switch (format) {
        case "tar":
            return "application/x-tar";
        case "tar.gz":
            return "application/gzip";
        case "zip":
            return "application/zip";
    }
}

export function archiveLabel(format: ArchiveFormat): string {
    switch (format) {
        case "tar":
            return "tar (uncompressed)";
        case "tar.gz":
            return "tar.gz (gzip-compressed tarball)";
        case "zip":
            return "zip (cross-platform)";
    }
}

// suggestedArchiveFilename names the saved archive without trying to
// be clever. A single-folder selection becomes "<folder>.tar.gz"; an
// empty / root path becomes "archive.tar.gz"; multiple selections —
// where pretending to be one folder would lie — become
// "selection.tar.gz".
export function suggestedArchiveFilename(
    name: string | string[],
    format: ArchiveFormat,
): string {
    const ext = archiveExtension(format);
    if (Array.isArray(name)) {
        if (name.length === 1) return suggestedArchiveFilename(name[0], format);
        return `selection${ext}`;
    }
    const trimmed = name.replace(/[\\/]+$/, "");
    if (!trimmed) return `archive${ext}`;
    return `${trimmed}${ext}`;
}
