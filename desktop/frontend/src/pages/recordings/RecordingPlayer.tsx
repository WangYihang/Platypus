import { useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";

import { palette, space } from "../../layout/theme";
import { fetchRecordingCastBlob } from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { loadAsciinemaPlayer } from "./asciinemaLoader";

interface Props {
    projectId: string;
    recordingId: string;
    cols?: number;
    rows?: number;
    autoPlay?: boolean;
}

// RecordingPlayer mounts an asciinema-player instance pointed at the
// recording's .cast bytes. The cast is fetched authenticated as a
// Blob (asciinema-player can't carry our Bearer header on its own
// fetch) and handed to the player via a blob: URL, which is revoked
// on unmount.
export default function RecordingPlayer({ projectId, recordingId, cols, rows, autoPlay = false }: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const playerRef = useRef<{ dispose?: () => void } | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        let blobUrl: string | null = null;
        setLoading(true);
        setError(null);

        (async () => {
            try {
                const [api, blob] = await Promise.all([
                    loadAsciinemaPlayer(),
                    fetchRecordingCastBlob(projectId, recordingId),
                ]);
                if (cancelled || !containerRef.current) return;
                blobUrl = URL.createObjectURL(blob);
                playerRef.current = api.create(blobUrl, containerRef.current, {
                    autoPlay,
                    cols,
                    rows,
                    fit: "width",
                    idleTimeLimit: 2,
                    theme: "asciinema",
                });
                setLoading(false);
            } catch (e) {
                if (cancelled) return;
                setError(humanizeError(e));
                setLoading(false);
            }
        })();

        return () => {
            cancelled = true;
            if (playerRef.current?.dispose) {
                try {
                    playerRef.current.dispose();
                } catch {
                    // dispose can throw if the player hasn't fully
                    // attached yet; we're tearing down anyway.
                }
            }
            playerRef.current = null;
            if (blobUrl) URL.revokeObjectURL(blobUrl);
            // Reset the container so a re-mount starts clean.
            if (containerRef.current) containerRef.current.innerHTML = "";
        };
    }, [projectId, recordingId, cols, rows, autoPlay]);

    return (
        <div style={{ position: "relative" }}>
            {loading && (
                <div
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        gap: space[2],
                        color: palette.textMuted,
                        fontSize: 12,
                        zIndex: 1,
                    }}
                >
                    <Loader2 className="size-4 animate-spin" />
                    Loading player…
                </div>
            )}
            {error && (
                <div
                    style={{
                        padding: `${space[3]}px ${space[4]}px`,
                        color: palette.danger,
                        fontSize: 13,
                        background: palette.surface,
                        borderRadius: 6,
                    }}
                >
                    {error}
                </div>
            )}
            <div
                ref={containerRef}
                style={{
                    width: "100%",
                    minHeight: 360,
                    background: "#1c1c1c",
                    borderRadius: 6,
                }}
            />
        </div>
    );
}
