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
    EnrollmentTokenListItem,
    IssueEnrollmentTokenResponse,
    listEnrollmentTokens,
    revokeEnrollmentToken,
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
import IssuePATDialog from "./IssuePATDialog";
import IssuedPATDialog from "./IssuedPATDialog";

interface Props {
    projectID: string;
}

// PATPanel is the management view for raw enrollment tokens —
// long-lived credentials a CI / scripted flow can submit at /enroll
// without going through the install-command shape. The panel mirrors
// InstallPanel structurally so the two tabs feel like siblings.
export default function PATPanel({ projectID }: Props) {
    const queryClient = useQueryClient();
    const [filter, setFilter] = useState<"active" | "all">("active");
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] =
        useState<IssueEnrollmentTokenResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] =
        useState<EnrollmentTokenListItem | null>(null);

    const enrollmentTokensKey = ["enrollmentTokens", projectID, filter] as const;
    const {
        data: rows = null,
        error,
        isFetching: loading,
    } = useQuery({
        queryKey: enrollmentTokensKey,
        queryFn: () => listEnrollmentTokens(projectID, filter === "all"),
    });
    const refresh = useCallback(() => {
        queryClient.invalidateQueries({ queryKey: enrollmentTokensKey });
    }, [queryClient, enrollmentTokensKey]);

    useEffect(() => {
        if (error) {
            toast.error(`Couldn't load enrollment tokens: ${humanizeError(error)}`);
        }
    }, [error]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokeEnrollmentToken(projectID, r.token_id);
            toast.success("Enrollment token revoked");
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
                            Issue enrollment token
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
                        title="No enrollment tokens issued yet"
                        description="Prefer the install command tab unless you need raw enrollment tokens for a CI pipeline."
                    />
                ) : (
                    <PATTable
                        rows={rows}
                        onRevoke={(r) => setPendingRevoke(r)}
                    />
                )}
            </Card>

            <IssuePATDialog
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
            <IssuedPATDialog result={lastIssued} onClose={() => setLastIssued(null)} />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke enrollment token?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The token will be rejected on any subsequent enrollment
                            attempt.
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

function PATTable({
    rows,
    onRevoke,
}: {
    rows: EnrollmentTokenListItem[];
    onRevoke: (r: EnrollmentTokenListItem) => void;
}) {
    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead className="w-[260px]">Token ID</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead className="w-[110px]">Status</TableHead>
                    <TableHead className="w-[80px]">Uses</TableHead>
                    <TableHead className="w-[120px]">Expires</TableHead>
                    <TableHead className="w-[80px]" />
                </TableRow>
            </TableHeader>
            <TableBody>
                {rows.map((r) => (
                    <TableRow key={r.token_id}>
                        <TableCell>
                            <Mono>{r.token_id}</Mono>
                        </TableCell>
                        <TableCell>
                            {r.description || (
                                <span className="text-text-muted">—</span>
                            )}
                        </TableCell>
                        <TableCell>
                            <StatusBadge status={r.status} />
                        </TableCell>
                        <TableCell>
                            {r.uses}/{r.max_uses}
                        </TableCell>
                        <TableCell>{fromNow(r.expires_at)}</TableCell>
                        <TableCell>
                            {!r.revoked && r.status === "pending" && (
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
