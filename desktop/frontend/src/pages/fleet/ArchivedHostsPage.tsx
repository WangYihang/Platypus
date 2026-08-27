import { useState } from "react";
import { ArchiveRestore, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import RefreshButton from "../../components/RefreshButton";
import { useCurrentProject } from "../../layout/ProjectShell";
import { palette, space } from "../../layout/theme";
import { Host, listArchivedHosts, restoreHost } from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

// ArchivedHostsPage lists the project's archived hosts and offers the
// one action that matters here: putting one back.
//
// Archiving is a soft delete — the row keeps its recordings, security
// scans and config audits — so this page is the other half of that
// promise. Without somewhere to see archived rows, "reversible" is
// only true in the database.
//
// Note that a host whose agent re-enrols un-archives itself, so rows
// that linger here are machines that are genuinely gone or offline.
export default function ArchivedHostsPage() {
    const project = useCurrentProject();
    const qc = useQueryClient();
    const [restoring, setRestoring] = useState<string | null>(null);

    const { data, isLoading, isFetching, refetch } = useQuery({
        queryKey: qk.archivedHosts(project.id),
        queryFn: () => listArchivedHosts(project.id),
    });
    const hosts = data ?? [];

    const restoreMu = useMutation({
        mutationFn: (hid: string) => restoreHost(project.id, hid),
        onMutate: (hid) => setRestoring(hid),
        onSettled: () => setRestoring(null),
        onSuccess: () => {
            toast.success("Host restored");
            void qc.invalidateQueries({ queryKey: qk.archivedHosts(project.id) });
            void qc.invalidateQueries({ queryKey: qk.hosts(project.id) });
        },
        onError: (e) => toast.error(humanizeError(e)),
    });

    if (isLoading) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <div
                style={{
                    flexShrink: 0,
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    padding: `${space[2]}px ${space[4]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                    background: palette.rail,
                    fontSize: 12,
                    color: palette.textMuted,
                }}
            >
                <span data-testid="archived-hosts-count">
                    {hosts.length} archived {hosts.length === 1 ? "host" : "hosts"}
                </span>
                <RefreshButton loading={isFetching} onClick={() => void refetch()} />
            </div>

            {hosts.length === 0 ? (
                <EmptyState
                    title="Nothing archived"
                    description="Archived hosts drop out of the fleet list but keep their recordings, scans and audit history. Archive one from its host page and it will show up here."
                />
            ) : (
                <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
                    <Table data-testid="archived-hosts-table">
                        <TableHeader>
                            <TableRow>
                                <TableHead>Host</TableHead>
                                <TableHead>OS</TableHead>
                                <TableHead>Archived</TableHead>
                                <TableHead>Reason</TableHead>
                                <TableHead />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {hosts.map((h: Host) => (
                                <TableRow key={h.id}>
                                    <TableCell>
                                        <Mono>{h.hostname || h.id}</Mono>
                                    </TableCell>
                                    <TableCell>{h.os || "—"}</TableCell>
                                    <TableCell>
                                        {h.archived_at
                                            ? new Date(h.archived_at).toLocaleString()
                                            : "—"}
                                    </TableCell>
                                    <TableCell style={{ color: palette.textMuted }}>
                                        {h.archived_reason || "—"}
                                    </TableCell>
                                    <TableCell style={{ textAlign: "right" }}>
                                        <Button
                                            size="sm"
                                            variant="outline"
                                            disabled={restoring === h.id}
                                            onClick={() => restoreMu.mutate(h.id)}
                                            aria-label={`Restore ${h.hostname || h.id}`}
                                        >
                                            {restoring === h.id ? (
                                                <Loader2 className="size-3.5 animate-spin" />
                                            ) : (
                                                <ArchiveRestore className="size-3.5" />
                                            )}
                                            Restore
                                        </Button>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
            )}
        </div>
    );
}
