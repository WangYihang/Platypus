import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Trash2 } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import Card from "../../../components/Card";
import EmptyState from "../../../components/EmptyState";
import Mono from "../../../components/Mono";
import RefreshButton from "../../../components/RefreshButton";
import Toolbar from "../../../components/Toolbar";
import { palette, space } from "../../../layout/theme";
import {
    InstallArtifactListItem,
    IssueInstallResponse,
    listInstallArtifacts,
    revokeInstallArtifact,
} from "../../../lib/api";
import { humanizeError } from "../../../lib/humanizeError";
import { fromNow } from "../../../lib/time";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import StatusBadge from "../StatusBadge";
import IssueInstallDialog from "./IssueInstallDialog";
import IssuedInstallDialog from "./IssuedInstallDialog";

interface Props {
    projectID: string;
    projectSlug: string;
}

// InstallPanel is the management view for issued install commands —
// the one-shot `curl ... | sh` artifacts an admin can hand out to a
// host so it can self-bootstrap. Lists active (or all) artifacts
// with an inline revoke control; "Generate install command" opens
// the issue dialog and on success pops the result dialog with the
// copyable command.
export default function InstallPanel({ projectID, projectSlug }: Props) {
    const queryClient = useQueryClient();
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueInstallResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] =
        useState<InstallArtifactListItem | null>(null);

    const installArtifactsKey = ["installArtifacts", projectID, filter] as const;
    const {
        data: rows = null,
        error,
        isFetching: loading,
    } = useQuery({
        queryKey: installArtifactsKey,
        queryFn: () => listInstallArtifacts(projectID, filter === "all"),
    });
    const refresh = useCallback(() => {
        queryClient.invalidateQueries({ queryKey: installArtifactsKey });
    }, [queryClient, installArtifactsKey]);

    useEffect(() => {
        if (error) {
            toast.error(`Couldn't load install commands: ${humanizeError(error)}`);
        }
    }, [error]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokeInstallArtifact(projectID, r.download_id);
            toast.success("Install link revoked");
            refresh();
        } catch (e) {
            toast.error(`Couldn't revoke: ${humanizeError(e)}`);
        }
    }

    return (
        <>
            <Toolbar
                left={
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={filter}
                        onValueChange={(v) => {
                            if (v) setFilter(v as "active" | "all");
                        }}
                    >
                        <ToggleGroupItem value="active">Active</ToggleGroupItem>
                        <ToggleGroupItem value="all">All</ToggleGroupItem>
                    </ToggleGroup>
                }
                right={
                    <>
                        <RefreshButton loading={loading} onClick={refresh} />
                        <Button size="sm" onClick={() => setIssueOpen(true)}>
                            <Plus className="size-3.5" />
                            Generate install command
                        </Button>
                    </>
                }
            />
            {error && (
                <div
                    style={{
                        marginBottom: space[3],
                        padding: `${space[3]}px ${space[4]}px`,
                        border: `1px solid ${palette.danger}`,
                        borderRadius: 6,
                        color: palette.danger,
                        fontSize: 13,
                    }}
                >
                    {String(error)}
                </div>
            )}
            <Card padding={0}>
                {rows === null ? (
                    <div className="flex items-center justify-center p-10">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : rows.length === 0 ? (
                    <EmptyState
                        title="No install commands yet"
                        description="Generate one for a host in this project."
                    />
                ) : (
                    <InstallArtifactsTable
                        rows={rows}
                        onRevoke={(r) => setPendingRevoke(r)}
                    />
                )}
            </Card>

            <IssueInstallDialog
                open={issueOpen}
                onOpenChange={(o) => {
                    setIssueOpen(o);
                    if (!o) refresh();
                }}
                onIssued={(r) => {
                    setLastIssued(r);
                    setIssueOpen(false);
                    refresh();
                }}
                projectID={projectID}
            />
            <IssuedInstallDialog
                result={lastIssued}
                projectSlug={projectSlug}
                onClose={() => setLastIssued(null)}
            />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke install link?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The curl command will stop working immediately.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmRevoke}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Revoke
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}

function InstallArtifactsTable({
    rows,
    onRevoke,
}: {
    rows: InstallArtifactListItem[];
    onRevoke: (r: InstallArtifactListItem) => void;
}) {
    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead className="w-[260px]">Download ID</TableHead>
                    <TableHead className="w-[120px]">Target</TableHead>
                    <TableHead>Server</TableHead>
                    <TableHead className="w-[110px]">Status</TableHead>
                    <TableHead className="w-[120px]">Expires</TableHead>
                    <TableHead>Consumed</TableHead>
                    <TableHead className="w-[80px]" />
                </TableRow>
            </TableHeader>
            <TableBody>
                {rows.map((r) => (
                    <TableRow key={r.download_id}>
                        <TableCell>
                            <Mono>{r.download_id}</Mono>
                        </TableCell>
                        <TableCell>
                            {r.target_os || r.target_arch ? (
                                `${r.target_os || "any"}/${r.target_arch || "any"}`
                            ) : (
                                <span className="text-text-muted">—</span>
                            )}
                        </TableCell>
                        <TableCell>
                            <Mono>{r.server_endpoint}</Mono>
                        </TableCell>
                        <TableCell>
                            <StatusBadge status={r.status} />
                        </TableCell>
                        <TableCell>{fromNow(r.expires_at)}</TableCell>
                        <TableCell>
                            {r.consumed_at ? (
                                <span>
                                    {fromNow(r.consumed_at)}
                                    {r.consumed_ip ? ` · ${r.consumed_ip}` : ""}
                                </span>
                            ) : (
                                <span className="text-text-muted">—</span>
                            )}
                        </TableCell>
                        <TableCell>
                            {!r.revoked && !r.consumed_at && (
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                    onClick={() => onRevoke(r)}
                                >
                                    <Trash2 className="size-3.5" />
                                </Button>
                            )}
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
        </Table>
    );
}
