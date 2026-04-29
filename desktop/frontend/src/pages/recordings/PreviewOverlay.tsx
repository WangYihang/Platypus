import { useEffect } from "react";
import { Download, X } from "lucide-react";

import Mono from "../../components/Mono";
import { palette, radius, space } from "../../layout/theme";
import { TerminalRecording } from "../../lib/api";
import { formatBytes } from "../../lib/format";
import { Button } from "@/components/ui/button";

import RecordingPlayer from "./RecordingPlayer";
import { formatDuration } from "./format";

interface Props {
    rec: TerminalRecording;
    projectId: string;
    onClose: () => void;
    onDownload: () => void;
}

// Vanilla fixed-position modal — Radix Dialog's focus trap interacts
// badly with the asciinema player's keyboard handler and freezes the
// page. We manage just Escape-to-close + body scroll lock.
export default function PreviewOverlay({ rec, projectId, onClose, onDownload }: Props) {
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") onClose();
        };
        document.addEventListener("keydown", onKey);
        const prevOverflow = document.body.style.overflow;
        document.body.style.overflow = "hidden";
        return () => {
            document.removeEventListener("keydown", onKey);
            document.body.style.overflow = prevOverflow;
        };
    }, [onClose]);

    const hostLabel = rec.host_alias || rec.host_hostname || rec.host_id || "—";

    return (
        <div
            onClick={onClose}
            style={{
                position: "fixed",
                inset: 0,
                background: "rgba(0,0,0,0.7)",
                zIndex: 50,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: space[6],
            }}
        >
            <div
                onClick={(e) => e.stopPropagation()}
                style={{
                    width: "min(960px, 100%)",
                    maxHeight: "90vh",
                    overflow: "auto",
                    background: palette.surface,
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.md,
                    display: "flex",
                    flexDirection: "column",
                }}
            >
                <div
                    style={{
                        display: "flex",
                        alignItems: "flex-start",
                        gap: space[3],
                        padding: `${space[4]}px ${space[5]}px`,
                        borderBottom: `1px solid ${palette.border}`,
                    }}
                >
                    <div style={{ flex: 1, minWidth: 0 }}>
                        <div
                            style={{
                                fontWeight: 600,
                                fontSize: 14,
                                color: palette.textPrimary,
                                marginBottom: 4,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                            }}
                            title={rec.title || rec.id}
                        >
                            {rec.title || <Mono>{rec.id.slice(0, 12)}</Mono>}
                        </div>
                        <div style={{ fontSize: 12, color: palette.textSecondary }}>
                            {rec.username ? `${rec.username} on ` : ""}
                            <Mono>{hostLabel}</Mono>
                            {` · ${formatDuration(rec.duration_ms)} · ${formatBytes(rec.size_bytes)}`}
                        </div>
                    </div>
                    <button
                        type="button"
                        onClick={onClose}
                        aria-label="Close"
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            justifyContent: "center",
                            width: 28,
                            height: 28,
                            borderRadius: 6,
                            border: "none",
                            background: "transparent",
                            color: palette.textSecondary,
                            cursor: "pointer",
                        }}
                    >
                        <X className="size-4" />
                    </button>
                </div>
                <div style={{ padding: space[4] }}>
                    <RecordingPlayer
                        projectId={projectId}
                        recordingId={rec.id}
                        autoPlay={false}
                    />
                </div>
                <div
                    style={{
                        display: "flex",
                        justifyContent: "flex-end",
                        gap: space[2],
                        padding: `${space[3]}px ${space[5]}px`,
                        borderTop: `1px solid ${palette.border}`,
                    }}
                >
                    <Button variant="outline" onClick={onDownload}>
                        <Download className="size-3.5" /> Download .cast
                    </Button>
                    <Button onClick={onClose}>Close</Button>
                </div>
            </div>
        </div>
    );
}
