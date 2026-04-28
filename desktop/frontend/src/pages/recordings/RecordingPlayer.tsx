import { useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";

import { palette, space } from "../../layout/theme";
import { authFetch } from "../../lib/auth";
import { humanizeError } from "../../lib/humanizeError";
import { loadAsciinemaPlayer } from "./asciinemaLoader";

interface Props {
    projectId: string;
    recordingId: string;
    autoPlay?: boolean;
}

// RecordingPlayer mounts an asciinema-player instance that plays back
// the recording's .cast bytes. Implementation details that took some
// hunting to get stable:
//
//   - The cast is fetched as TEXT (via authFetch) and handed to the
//     player as `{ data: text }`. The earlier blob-URL approach made
//     the player's internal fetch race with our blob lifecycle and
//     in some browsers triggered range requests that hung the page
//     once playback started.
//   - We pass NO terminal-size options. The cast file's header has
//     authoritative cols/rows; passing different ones via options
//     can cause the player's auto-fit pass to thrash on layout.
//   - We pass NO fit / theme / idleTimeLimit options. Defaults are
//     fine and the explicit values were correlated with the runaway
//     CPU loop reported on the Recordings page.
//   - We do NOT host this inside a Radix Dialog. The dialog's focus
//     trap + the player's keyboard handler interact poorly and the
//     page becomes unresponsive once playback starts. The parent
//     opens us inside a plain fixed-position overlay instead.
export default function RecordingPlayer({ projectId, recordingId, autoPlay = false }: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const playerRef = useRef<{ dispose?: () => void } | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        setError(null);

        (async () => {
            try {
                const [api, resp] = await Promise.all([
                    loadAsciinemaPlayer(),
                    authFetch(
                        `/api/v1/projects/${projectId}/recordings/${recordingId}/cast`,
                    ),
                ]);
                const text = await resp.text();
                if (cancelled || !containerRef.current) return;

                playerRef.current = api.create(
                    { data: text },
                    containerRef.current,
                    { autoPlay },
                );
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
            // Reset the container so a re-mount starts clean.
            if (containerRef.current) containerRef.current.innerHTML = "";
        };
    }, [projectId, recordingId, autoPlay]);

    return (
        <div style={{ position: "relative", width: "100%" }}>
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
                        background: "#1c1c1c",
                        borderRadius: 6,
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
                }}
            />
        </div>
    );
}
