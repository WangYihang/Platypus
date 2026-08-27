import { Loader2 } from "lucide-react";

import { humanize } from "../../../lib/format";
import { useRemotePreviewURL } from "./remoteFile";

interface Props {
    projectID: string;
    sessionHash: string;
    path: string;
    size: number;
    kind: "video" | "audio";
    mime?: string;
}

// MediaViewer renders <video> / <audio> for video and audio files. In
// web mode it mints a short-lived preview URL and hands it to the
// native element so the browser can issue Range requests directly —
// scrubbing past the first KB doesn't force a full re-download. In
// desktop / non-web mode (no preview-token endpoint over Wails IPC)
// it falls back to the legacy "load all bytes via ReadFile + Blob URL"
// path; that's still acceptable because desktop file access is local
// and a full read is cheap.
export default function MediaViewer({
    projectID,
    sessionHash,
    path,
    size,
    kind,
    mime,
}: Props) {
    const { url, error: loadError } = useRemotePreviewURL({
        projectID,
        sessionHash,
        path,
        mime: mime || (kind === "video" ? "video/*" : "audio/*"),
    });
    const error = loadError
        ? loadError instanceof Error
            ? loadError.message
            : String(loadError)
        : null;

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
                    // preload="metadata" pulls just the head/MOOV so
                    // duration + dimensions render without
                    // downloading the whole payload — the Range
                    // pipeline does the rest as the user seeks.
                    <video
                        src={url}
                        controls
                        preload="metadata"
                        className="max-h-full max-w-full"
                    />
                ) : (
                    <audio
                        src={url}
                        controls
                        preload="metadata"
                        className="w-full max-w-xl"
                    />
                )}
            </div>
        </div>
    );
}
