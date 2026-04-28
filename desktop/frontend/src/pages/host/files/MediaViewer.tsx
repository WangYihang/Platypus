import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";

import { ReadFile } from "@wails/go/app/App";
import { humanize } from "../../../lib/format";

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
    kind: "video" | "audio";
    mime?: string;
}

function bytesFromWailsRead(raw: unknown): Uint8Array {
    if (raw instanceof Uint8Array) return raw;
    if (Array.isArray(raw)) return new Uint8Array(raw as number[]);
    throw new Error(`unexpected ReadFile shape: ${typeof raw}`);
}

// MediaViewer fetches the bytes via the existing ReadFile pipe, builds
// a blob URL, and hands it to the native <video> / <audio> element.
// Whole-file load is fine for the file sizes the operator typically
// browses; a follow-up could move to a Range-supporting endpoint for
// streaming if multi-GB media becomes a regular case.
export default function MediaViewer({
    projectID,
    sessionHash,
    path,
    size,
    kind,
    mime,
}: Props) {
    const [url, setUrl] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        let createdURL: string | null = null;
        setUrl(null);
        setError(null);
        (async () => {
            try {
                const raw = await ReadFile(projectID, sessionHash, path, 0, 0);
                if (cancelled) return;
                const bytes = bytesFromWailsRead(raw);
                const blob = new Blob([bytes as BlobPart], {
                    type: mime || (kind === "video" ? "video/*" : "audio/*"),
                });
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
    }, [projectID, sessionHash, path, kind, mime]);

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
                    {mime || kind} · {humanize(size)}
                </div>
            </div>
            <div className="flex flex-1 items-center justify-center overflow-auto bg-[color:var(--muted)] p-4">
                {kind === "video" ? (
                    <video src={url} controls className="max-h-full max-w-full" />
                ) : (
                    <audio src={url} controls className="w-full max-w-xl" />
                )}
            </div>
        </div>
    );
}
