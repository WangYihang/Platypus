import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react";
import { toast } from "sonner";

import EmptyState from "../components/EmptyState";
import FilterToolbar from "../components/FilterToolbar";
import { icons } from "../lib/icons";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    RecordingStatus,
    TerminalRecording,
    deleteRecording,
    listRecordings,
    updateRecording,
} from "../lib/api";
import { getSession } from "../lib/auth";
import { humanizeError } from "../lib/humanizeError";

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

import PreviewOverlay from "./recordings/PreviewOverlay";
import RecordingCard from "./recordings/RecordingCard";

const PAGE_SIZE = 24;

// /projects/:slug/recordings — paginated card grid of terminal session
// recordings. Card → preview opens an asciinema player in a vanilla
// modal (PreviewOverlay).
export default function RecordingsPage() {
    const project = useCurrentProject();
    const queryClient = useQueryClient();
    const [pageStack, setPageStack] = useState<(string | undefined)[]>([undefined]);

    const [statusFilter, setStatusFilter] = useState<RecordingStatus | "">("");
    const [query, setQuery] = useState("");
    const [debouncedQuery, setDebouncedQuery] = useState("");

    const [previewing, setPreviewing] = useState<TerminalRecording | null>(null);
    const [renaming, setRenaming] = useState<TerminalRecording | null>(null);
    const [renameDraft, setRenameDraft] = useState("");
    const [deleting, setDeleting] = useState<TerminalRecording | null>(null);

    // Debounce search so each keystroke doesn't fire a list request.
    useEffect(() => {
        const t = setTimeout(() => setDebouncedQuery(query.trim()), 250);
        return () => clearTimeout(t);
    }, [query]);

    const currentCursor = pageStack[pageStack.length - 1];
    const pageNumber = pageStack.length;

    const recordingsKey = [
        "recordings",
        project.id,
        { cursor: currentCursor ?? null, status: statusFilter || null, q: debouncedQuery || null },
    ] as const;

    const recordingsQuery = useQuery({
        queryKey: recordingsKey,
        queryFn: () =>
            listRecordings(project.id, {
                cursor: currentCursor,
                limit: PAGE_SIZE,
                status: statusFilter || undefined,
                q: debouncedQuery || undefined,
            }),
    });
    const items = recordingsQuery.data?.items ?? null;
    const total = recordingsQuery.data?.total ?? 0;
    const nextCursor = recordingsQuery.data?.next_cursor ?? null;
    const loading = recordingsQuery.isFetching;
    const error = recordingsQuery.error;

    const refresh = useCallback(
        (_cursor?: string) =>
            queryClient.invalidateQueries({ queryKey: recordingsKey }),
        [queryClient, recordingsKey],
    );

    useEffect(() => {
        if (error) toast.error(`load recordings: ${humanizeError(error)}`);
    }, [error]);

    // Reset to page 1 whenever the filter or query changes.
    useEffect(() => {
        setPageStack([undefined]);
    }, [statusFilter, debouncedQuery, project.id]);

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

    // <a download> can't carry an Authorization header, so we fetch the blob
    // and trigger a synthetic anchor click.
    const downloadRecording = async (rec: TerminalRecording) => {
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
        <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
            <FilterToolbar
                search={{
                    value: query,
                    onChange: setQuery,
                    placeholder: "Search title / shell / host",
                    minWidth: 280,
                }}
                filters={
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
                }
                count={
                    items === null
                        ? "Loading…"
                        : `${total.toLocaleString()} session${total === 1 ? "" : "s"}${
                              totalPagesHint > 1
                                  ? ` · page ${pageNumber} of ${totalPagesHint}`
                                  : ""
                          }`
                }
                refreshLoading={loading}
                onRefresh={() => refresh(currentCursor)}
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
                        {humanizeError(error)}
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
                                projectId={project.id}
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

            {previewing && (
                <PreviewOverlay
                    rec={previewing}
                    projectId={project.id}
                    onClose={() => setPreviewing(null)}
                    onDownload={() => downloadRecording(previewing)}
                />
            )}

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
