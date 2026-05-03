import { useEffect, useRef, useState } from "react";

import { authFetch } from "../../lib/auth";
import { palette, radius } from "../../layout/theme";
import { loadAsciinemaPlayer } from "./asciinemaLoader";

interface Props {
    projectId: string;
    recordingId: string;
    // Card click → open the full-size preview modal. Clicking the
    // thumbnail itself routes through here too (the inner player is
    // pointer-events:none so it can't intercept).
    onClick: () => void;
    // Aspect ratio (width / height) for the thumbnail box. Defaults to
    // 16:9 which matches how a typical terminal session reads when
    // squashed into a card.
    aspect?: number;
}

// RecordingThumbnail mounts a chrome-less asciinema-player inside a
// recording card so the operator can recognise the session at a
// glance instead of staring at six lines of metadata. Implementation
// details:
//
//   · Lazy-mount via IntersectionObserver. Listing pages can carry
//     20+ cards and we don't want to fetch + parse + render N cast
//     files for cards the operator never scrolls to. The placeholder
//     stays in the layout so the card height doesn't jump when the
//     player attaches.
//
//   · `poster: "npt:0:1.5"` replays the first 1.5 s of the cast
//     and renders the resulting terminal as a static frame. That's
//     usually the prompt + first command, which is the most
//     recognisable thumbnail content for a shell session. Without
//     `poster` the player paints a black grid until the first play
//     click — useless as a preview.
//
//   · v3 doesn't expose a `controls: false` option, so the bottom
//     control bar AND the `.ap-overlay-start` big-play SVG are
//     suppressed via the scoped CSS at the bottom of this file. We
//     also clamp `pointer-events: none` on the player so its own
//     click-to-play handler can't fire — the card-level button owns
//     the gesture.
//
//   · We do NOT pass `fit: "width"`. The player's auto-fit pass
//     interferes with the wrapper's overflow-clip, leaving a
//     letterbox; sizing the wrapper and letting the player render at
//     its natural cols/rows is more predictable across cast widths.
export default function RecordingThumbnail({
    projectId,
    recordingId,
    onClick,
    aspect = 16 / 9,
}: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const playerRef = useRef<{ dispose?: () => void } | null>(null);
    const [visible, setVisible] = useState(false);
    const [failed, setFailed] = useState(false);

    // 1) Lazy-mount: flip `visible` on the first IntersectionObserver
    //    fire that says "in viewport". Disconnect immediately after
    //    so the heavy load only ever happens once per card.
    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;
        if (typeof IntersectionObserver === "undefined") {
            // SSR / very old browsers — just mount eagerly.
            setVisible(true);
            return;
        }
        const io = new IntersectionObserver(
            (entries) => {
                for (const e of entries) {
                    if (e.isIntersecting) {
                        setVisible(true);
                        io.disconnect();
                        return;
                    }
                }
            },
            // 200 px rootMargin pre-mounts cards just below the
            // fold so they're ready by the time scroll reveals them.
            { rootMargin: "200px" },
        );
        io.observe(el);
        return () => io.disconnect();
    }, []);

    // 2) Once visible, fetch the cast and create a player with the
    //    poster baked in. Standard cleanup on unmount / re-mount.
    useEffect(() => {
        if (!visible) return;
        let cancelled = false;
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
                    {
                        autoPlay: false,
                        poster: "npt:0:1.5",
                    },
                );
            } catch {
                if (cancelled) return;
                setFailed(true);
            }
        })();
        return () => {
            cancelled = true;
            try {
                playerRef.current?.dispose?.();
            } catch {
                /* ignore */
            }
            playerRef.current = null;
            if (containerRef.current) containerRef.current.innerHTML = "";
        };
    }, [visible, projectId, recordingId]);

    return (
        <button
            type="button"
            className="recording-thumbnail"
            onClick={onClick}
            aria-label="Preview recording"
            style={{
                position: "relative",
                display: "block",
                width: "100%",
                aspectRatio: String(aspect),
                background: palette.main,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.sm,
                overflow: "hidden",
                cursor: "pointer",
                padding: 0,
            }}
        >
            <div
                ref={containerRef}
                style={{
                    position: "absolute",
                    inset: 0,
                    // Block the inner player from swallowing the
                    // click — the card-level button owns the gesture.
                    pointerEvents: "none",
                    // The player paints a control bar + start-overlay
                    // we don't want here. Hide them with scoped CSS
                    // selectors so we don't disturb the full-size
                    // preview's chrome elsewhere on the page.
                }}
            />
            {failed && (
                <span
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        color: palette.textMuted,
                        fontSize: 11,
                    }}
                >
                    preview unavailable
                </span>
            )}
            {/* Scoped controls hider. The full-size player keeps its
                chrome because RecordingPlayer doesn't render under
                `.recording-thumbnail`. */}
            <style>{`
                .recording-thumbnail .ap-control-bar { display: none !important; }
                .recording-thumbnail .ap-overlay-start { display: none !important; }
                .recording-thumbnail .ap-wrapper { width: 100%; height: 100%; }
                .recording-thumbnail .ap-player { width: 100% !important; height: 100% !important; }
            `}</style>
        </button>
    );
}
