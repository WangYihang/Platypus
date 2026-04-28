import { useCallback, useEffect, useMemo, useState } from "react";
import {
    ChevronLeft,
    ChevronRight,
    Download,
    Loader2,
    Pencil,
    Play,
    RotateCw,
    Search,
    Trash2,
} from "lucide-react";
import { toast } from "sonner";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { icons } from "../lib/icons";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, radius, space } from "../layout/theme";
import {
    RecordingStatus,
    TerminalRecording,
    deleteRecording,
    listRecordings,
    updateRecording,
} from "../lib/api";
import { getSession } from "../lib/auth";
import { humanizeError } from "../lib/humanizeError";
import { formatBytes } from "../lib/format";
import { fromNow } from "../lib/time";

import RecordingPlayer from "./recordings/RecordingPlayer";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";

const PAGE_SIZE = 24;

const STATUS_TONE: Record<RecordingStatus, "success" | "warning" | "danger" | "neutral"> = {
    completed: "success",
    recording: "warning",
    failed: "danger",
};

// formatDuration renders a millisecond count as "1m 23s" / "12.4s" /
// "230ms" so the card stays scannable. Keeps the same visual weight
// across every status.
function formatDuration(ms: number): string {
    if (!Number.isFinite(ms) || ms < 0) return "—";
    if (ms < 1000) return `${ms}ms`;
    const totalSecs = ms / 1000;
    if (totalSecs < 60) return `${totalSecs.toFixed(1)}s`;
    const m = Math.floor(totalSecs / 60);
    const s = Math.floor(totalSecs % 60);
    return s === 0 ? `${m}m` : `${m}m ${s}s`;
}

// RecordingsPage renders /projects/:slug/recordings: a paginated card
// grid of terminal session recordings. Each card shows the operator,
// host, duration, size, and status; clicking opens an asciinema-player
// preview in a modal so the operator can scrub through the captured
// shell session.
export default function RecordingsPage() {
    const project = useCurrentProject();
    const [items, setItems] = useState<TerminalRecording[] | null>(null);
    const [total, setTotal] = useState<number>(0);
    const [pageStack, setPageStack] = useState<(string | undefined)[]>([undefined]);
    const [nextCursor, setNextCursor] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const [statusFilter, setStatusFilter] = useState<RecordingStatus | "">("");
    const [query, setQuery] = useState("");
    const [debouncedQuery, setDebouncedQuery] = useState("");

    const [previewing, setPreviewing] = useState<TerminalRecording | null>(null);
    const [renaming, setRenaming] = useState<TerminalRecording | null>(null);
    const [renameDraft, setRenameDraft] = useState("");
    const [deleting, setDeleting] = useState<TerminalRecording | null>(null);

    // Debounce the search box so each keystroke doesn't fire a list
    // request — feels jankier on every backend than just letting the
    // user type and refreshing once they've stopped.
    useEffect(() => {
        const t = setTimeout(() => setDebouncedQuery(query.trim()), 250);
        return () => clearTimeout(t);
    }, [query]);

    const currentCursor = pageStack[pageStack.length - 1];
    const pageNumber = pageStack.length;

    const refresh = useCallback(
        async (cursor: string | undefined) => {
            setLoading(true);
            try {
                const resp = await listRecordings(project.id, {
                    cursor,
                    limit: PAGE_SIZE,
                    status: statusFilter || undefined,
                    q: debouncedQuery || undefined,
                });
                setItems(resp.items);
                setTotal(resp.total);
                setNextCursor(resp.next_cursor ?? null);
                setError(null);
            } catch (e) {
                setError(humanizeError(e));
                toast.error(`load recordings: ${humanizeError(e)}`);
            } finally {
                setLoading(false);
            }
        },
        [project.id, statusFilter, debouncedQuery],
    );

    // Reset to page 1 whenever the filter or query changes — otherwise
    // the cursor stack would carry stale offsets that don't match the
    // new result set.
    useEffect(() => {
        setPageStack([undefined]);
    }, [statusFilter, debouncedQuery, project.id]);

    useEffect(() => {
        refresh(currentCursor);
    }, [refresh, currentCursor]);

    const totalPagesHint = useMemo(() => {
        if (total <= PAGE_SIZE) return 1;
        return Math.ceil(total / PAGE_SIZE);
    }, [total]);

    const handleNextPage = () => {
        if (!nextCursor) return;
        setPageStack((p) => [...p, nextCursor]);
    };

    const handlePrevPage = () => {
        if (pageStack.length <= 1) return;
        setPageStack((p) => p.slice(0, -1));
    };

    const handleDelete = async (rec: TerminalRecording) => {
        try {
            await deleteRecording(project.id, rec.id);
            toast.success("Recording deleted");
            setDeleting(null);
            // Refetch the current page so the grid reflects the
            // deletion. Stays on the same page; if it was the last
            // item the pagination falls back via the previous-page
            // button.
            refresh(currentCursor);
        } catch (e) {
            toast.error(`delete: ${humanizeError(e)}`);
        }
    };

    const handleRename = async () => {
        if (!renaming) return;
        try {
            await updateRecording(project.id, renaming.id, { title: renameDraft });
            toast.success("Recording renamed");
            setRenaming(null);
            refresh(currentCursor);
        } catch (e) {
            toast.error(`rename: ${humanizeError(e)}`);
        }
    };

    const downloadRecording = async (rec: TerminalRecording) => {
        // Build a server URL with download=1 so the browser presents
        // the file as a save dialog. The browser carries the
        // Authorization header for `fetch`, but a direct <a download>
        // wouldn't — so we fetch as blob and trigger a synthetic
        // anchor click.
        try {
            const session = getSession();
            if (!session) throw new Error("not logged in");
            const r = await fetch(
                `${session.serverURL}/api/v1/projects/${project.id}/recordings/${rec.id}/cast?download=1`,
                {
                    headers: { Authorization: "Bearer " + session.sessionToken },
                },
            );
            if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
            const blob = await r.blob();
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = `${rec.title || rec.id}.cast`;
            a.click();
            URL.revokeObjectURL(url);
        } catch (e) {
            toast.error(`download: ${humanizeError(e)}`);
        }
    };

    const I = icons;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Recordings"
                subtitle={
                    items === null
                        ? "Loading…"
                        : `${total.toLocaleString()} session${total === 1 ? "" : "s"}`
                }
                actions={
                    <Button
                        size="sm"
                        variant="outline"
                        disabled={loading}
                        onClick={() => refresh(currentCursor)}
                    >
                        {loading ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <RotateCw className="size-3.5" />
                        )}
                        Refresh
                    </Button>
                }
            />
            <Toolbar
                left={
                    <>
                        <Select
                            value={statusFilter || "__all__"}
                            onValueChange={(v) =>
                                setStatusFilter(v === "__all__" ? "" : (v as RecordingStatus))
                            }
                        >
                            <SelectTrigger size="sm" className="min-w-[150px]">
                                <SelectValue placeholder="Status" />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="__all__">All statuses</SelectItem>
                                <SelectItem value="completed">completed</SelectItem>
                                <SelectItem value="recording">recording</SelectItem>
                                <SelectItem value="failed">failed</SelectItem>
                            </SelectContent>
                        </Select>
                        <div className="relative max-w-[320px] w-full">
                            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                            <Input
                                placeholder="Search title / shell / host"
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                className="h-8 pl-8"
                            />
                        </div>
                    </>
                }
                right={
                    <span style={{ color: palette.textSecondary, fontSize: 12 }}>
                        Page {pageNumber}
                        {totalPagesHint > 1 ? ` of ${totalPagesHint}` : ""}
                    </span>
                }
            />

            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <div
                        style={{
                            marginBottom: space[4],
                            padding: `${space[3]}px ${space[4]}px`,
                            border: `1px solid ${palette.danger}`,
                            borderRadius: 6,
                            color: palette.danger,
                            fontSize: 13,
                        }}
                    >
                        {error}
                    </div>
                )}
                {!items && (
                    <div
                        style={{
                            display: "flex",
                            justifyContent: "center",
                            alignItems: "center",
                            padding: 80,
                        }}
                    >
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                )}
                {items && items.length === 0 && (
                    <EmptyState
                        icon={<I.shell className="size-5" />}
                        title="No recordings yet"
                        description="Every interactive shell opened from this project will appear here as an asciinema-compatible recording."
                    />
                )}
                {items && items.length > 0 && (
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))",
                            gap: space[4],
                        }}
                    >
                        {items.map((rec) => (
                            <RecordingCard
                                key={rec.id}
                                rec={rec}
                                onPreview={() => setPreviewing(rec)}
                                onRename={() => {
                                    setRenameDraft(rec.title || "");
                                    setRenaming(rec);
                                }}
                                onDelete={() => setDeleting(rec)}
                                onDownload={() => downloadRecording(rec)}
                            />
                        ))}
                    </div>
                )}

                {items && items.length > 0 && (
                    <div
                        style={{
                            display: "flex",
                            justifyContent: "center",
                            alignItems: "center",
                            gap: space[3],
                            marginTop: space[6],
                        }}
                    >
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={pageStack.length <= 1 || loading}
                            onClick={handlePrevPage}
                        >
                            <ChevronLeft className="size-3.5" /> Previous
                        </Button>
                        <span style={{ fontSize: 12, color: palette.textSecondary }}>
                            Page {pageNumber}
                        </span>
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={!nextCursor || loading}
                            onClick={handleNextPage}
                        >
                            Next <ChevronRight className="size-3.5" />
                        </Button>
                    </div>
                )}
            </div>

            <Dialog open={previewing !== null} onOpenChange={(o) => !o && setPreviewing(null)}>
                <DialogContent className="sm:max-w-[920px]">
                    <DialogHeader>
                        <DialogTitle>
                            {previewing?.title || (
                                <Mono>{previewing?.id.slice(0, 12) ?? ""}</Mono>
                            )}
                        </DialogTitle>
                        <DialogDescription>
                            {previewing?.username
                                ? `${previewing.username} on `
                                : ""}
                            {previewing?.host_alias ||
                                previewing?.host_hostname ||
                                previewing?.host_id ||
                                "—"}
                            {previewing &&
                                ` · ${formatDuration(previewing.duration_ms)} · ${formatBytes(
                                    previewing.size_bytes,
                                )}`}
                        </DialogDescription>
                    </DialogHeader>
                    {previewing && (
                        <RecordingPlayer
                            projectId={project.id}
                            recordingId={previewing.id}
                            cols={previewing.cols}
                            rows={previewing.rows}
                            autoPlay={false}
                        />
                    )}
                    <DialogFooter>
                        <Button
                            variant="outline"
                            onClick={() => previewing && downloadRecording(previewing)}
                        >
                            <Download className="size-3.5" /> Download .cast
                        </Button>
                        <Button onClick={() => setPreviewing(null)}>Close</Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Dialog open={renaming !== null} onOpenChange={(o) => !o && setRenaming(null)}>
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>Rename recording</DialogTitle>
                        <DialogDescription>
                            Give this session a memorable label so you can find it later.
                        </DialogDescription>
                    </DialogHeader>
                    <Input
                        placeholder="rotating wp creds on web-04"
                        value={renameDraft}
                        onChange={(e) => setRenameDraft(e.target.value)}
                        autoFocus
                    />
                    <DialogFooter>
                        <Button variant="outline" onClick={() => setRenaming(null)}>
                            Cancel
                        </Button>
                        <Button onClick={handleRename}>Save</Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Dialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
                <DialogContent className="sm:max-w-[420px]">
                    <DialogHeader>
                        <DialogTitle>Delete recording?</DialogTitle>
                        <DialogDescription>
                            This removes the .cast file from disk and the audit row. This
                            action can't be undone.
                        </DialogDescription>
                    </DialogHeader>
                    <DialogFooter>
                        <Button variant="outline" onClick={() => setDeleting(null)}>
                            Cancel
                        </Button>
                        <Button
                            style={{
                                background: palette.danger,
                                color: "#fff",
                            }}
                            onClick={() => deleting && handleDelete(deleting)}
                        >
                            Delete
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </div>
    );
}

interface RecordingCardProps {
    rec: TerminalRecording;
    onPreview: () => void;
    onRename: () => void;
    onDelete: () => void;
    onDownload: () => void;
}

function RecordingCard({
    rec,
    onPreview,
    onRename,
    onDelete,
    onDownload,
}: RecordingCardProps) {
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
                    <Button
                        size="sm"
                        variant="outline"
                        onClick={onRename}
                        title="Rename"
                    >
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
                    <Button
                        size="sm"
                        variant="outline"
                        onClick={onDelete}
                        title="Delete"
                    >
                        <Trash2 className="size-3.5" />
                    </Button>
                </div>
            </div>
        </Card>
    );
}
