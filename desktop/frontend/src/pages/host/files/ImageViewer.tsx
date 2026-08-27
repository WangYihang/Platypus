import { Loader2 } from "lucide-react";

import { humanize } from "../../../lib/format";
import { useRemoteObjectURL } from "./remoteFile";

// 16 MiB caps the bytes the viewer is willing to base64 through React
// state. The blob path is fine for typical screenshots / icons; a
// multi-GB raw camera dump would otherwise stall the renderer for
// minutes before failing. Above this size we render a placeholder
// and tell the user to use the toolbar's Download action instead.
const MAX_INLINE_IMAGE_BYTES = 16 * 1024 * 1024;

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
    // Server-supplied MIME. Used directly for the Blob type when present
    // so SVG/PNG/etc. render with the right intrinsic handling.
    mime?: string;
}

export default function ImageViewer({ projectID, sessionHash, path, size, mime }: Props) {
    const tooLarge = size > MAX_INLINE_IMAGE_BYTES;

    const {
        url,
        error: loadError,
    } = useRemoteObjectURL({
        projectID,
        sessionHash,
        path,
        mime: mime || "image/*",
        enabled: !tooLarge,
    });
    const error = loadError
        ? loadError instanceof Error
            ? loadError.message
            : String(loadError)
        : null;

    if (tooLarge) {
        return (
            <div className="flex h-full flex-col">
                <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                    <div className="truncate font-mono">{path}</div>
                    <div className="text-xs text-muted-foreground">
                        {mime || "image"} · {humanize(size)}
                    </div>
                </div>
                <div className="flex flex-1 items-center justify-center px-4 text-center text-sm text-muted-foreground">
                    Image is {humanize(size)} — too large to preview inline.
                    Use the toolbar's Download action to save it locally.
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex h-full items-center justify-center px-4 text-center text-sm text-red-500">
                {error}
            </div>
        );
    }

    if (!url) {
        return (
            <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Loading {humanize(size)}…
            </div>
        );
    }

    return (
        <div className="flex h-full flex-col">
            <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                <div className="truncate font-mono">{path}</div>
                <div className="text-xs text-muted-foreground">
                    {mime || "image"} · {humanize(size)}
                </div>
            </div>
            <div className="flex flex-1 items-center justify-center overflow-auto bg-[color:var(--muted)] p-4">
                <img
                    src={url}
                    alt={path}
                    className="max-h-full max-w-full object-contain"
                />
            </div>
        </div>
    );
}
