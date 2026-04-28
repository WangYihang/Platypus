import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";

import { ReadFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";

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

function bytesFromWailsRead(raw: unknown): Uint8Array {
    if (raw instanceof Uint8Array) return raw;
    if (Array.isArray(raw)) return new Uint8Array(raw as number[]);
    throw new Error(`unexpected ReadFile shape: ${typeof raw}`);
}

export default function ImageViewer({ projectID, sessionHash, path, size, mime }: Props) {
    const [url, setUrl] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    const tooLarge = size > MAX_INLINE_IMAGE_BYTES;

    useEffect(() => {
        if (tooLarge) return;
        let cancelled = false;
        let createdURL: string | null = null;
        setUrl(null);
        setError(null);
        (async () => {
            try {
                const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                if (cancelled) return;
                const bytes = bytesFromWailsRead(raw);
                const blob = new Blob([bytes as BlobPart], { type: mime || "image/*" });
                createdURL = URL.createObjectURL(blob);
                setUrl(createdURL);
            } catch (err) {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : String(err));
            }
        })();
        return () => {
            cancelled = true;
            if (createdURL) URL.revokeObjectURL(createdURL);
        };
    }, [projectID, sessionHash, path, mime, tooLarge]);

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
