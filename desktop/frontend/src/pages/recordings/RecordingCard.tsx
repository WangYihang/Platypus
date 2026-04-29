import { Download, Pencil, Play, Trash2 } from "lucide-react";

import Card from "../../components/Card";
import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { icons } from "../../lib/icons";
import { palette, radius, space } from "../../layout/theme";
import { TerminalRecording } from "../../lib/api";
import { formatBytes } from "../../lib/format";
import { fromNow } from "../../lib/time";
import { Button } from "@/components/ui/button";

import { STATUS_TONE, formatDuration } from "./format";

interface Props {
    rec: TerminalRecording;
    onPreview: () => void;
    onRename: () => void;
    onDelete: () => void;
    onDownload: () => void;
}

export default function RecordingCard({
    rec,
    onPreview,
    onRename,
    onDelete,
    onDownload,
}: Props) {
    const hostLabel = rec.host_alias || rec.host_hostname || rec.host_id || "—";
    const tone = STATUS_TONE[rec.status] ?? "neutral";
    const I = icons;

    const title =
        rec.title ||
        (rec.shell ? rec.shell : "Terminal session") +
            ` · ${new Date(rec.started_at).toLocaleString()}`;

    return (
        <Card padding={0}>
            <div
                style={{
                    padding: `${space[4]}px ${space[5]}px ${space[3]}px`,
                    display: "flex",
                    flexDirection: "column",
                    gap: space[2],
                }}
            >
                <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                    <I.shell className="size-4" style={{ color: palette.textSecondary }} />
                    <div
                        style={{
                            fontWeight: 600,
                            fontSize: 13,
                            color: palette.textPrimary,
                            flex: 1,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                        }}
                        title={title}
                    >
                        {title}
                    </div>
                    <StatusPill tone={tone}>{rec.status}</StatusPill>
                </div>
                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: "auto 1fr",
                        rowGap: 4,
                        columnGap: space[3],
                        fontSize: 12,
                        color: palette.textSecondary,
                    }}
                >
                    <span style={{ color: palette.textMuted }}>Host</span>
                    <Mono>{hostLabel}</Mono>
                    <span style={{ color: palette.textMuted }}>Operator</span>
                    <span>{rec.username || <span style={{ color: palette.textMuted }}>—</span>}</span>
                    <span style={{ color: palette.textMuted }}>Duration</span>
                    <span>{formatDuration(rec.duration_ms)}</span>
                    <span style={{ color: palette.textMuted }}>Size</span>
                    <span>{formatBytes(rec.size_bytes)}</span>
                    <span style={{ color: palette.textMuted }}>Started</span>
                    <span title={new Date(rec.started_at).toLocaleString()}>
                        {fromNow(rec.started_at)}
                    </span>
                </div>
            </div>

            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: `${space[2]}px ${space[3]}px`,
                    borderTop: `1px solid ${palette.border}`,
                    background: palette.surface,
                    borderBottomLeftRadius: radius.md,
                    borderBottomRightRadius: radius.md,
                }}
            >
                <Button
                    size="sm"
                    variant="default"
                    disabled={rec.status === "recording"}
                    onClick={onPreview}
                    title={rec.status === "recording" ? "Recording in progress" : "Preview"}
                >
                    <Play className="size-3.5" /> Preview
                </Button>
                <div style={{ display: "flex", gap: space[1] }}>
                    <Button size="sm" variant="outline" onClick={onRename} title="Rename">
                        <Pencil className="size-3.5" />
                    </Button>
                    <Button
                        size="sm"
                        variant="outline"
                        disabled={rec.status === "recording"}
                        onClick={onDownload}
                        title="Download .cast"
                    >
                        <Download className="size-3.5" />
                    </Button>
                    <Button size="sm" variant="outline" onClick={onDelete} title="Delete">
                        <Trash2 className="size-3.5" />
                    </Button>
                </div>
            </div>
        </Card>
    );
}
