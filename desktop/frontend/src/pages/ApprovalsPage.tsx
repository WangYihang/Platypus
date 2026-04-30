import { useState } from "react";
import { CheckCircle2, Loader2, ShieldX, XCircle } from "lucide-react";
import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import RemoteAddr from "../components/RemoteAddr";
import RefreshButton from "../components/RefreshButton";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    Host,
    approveHost,
    listPendingApprovals,
    rejectHost,
} from "../lib/api";
import { humanizeError } from "../lib/humanizeError";
import { qk } from "../lib/queryKeys";
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
import { Input } from "@/components/ui/input";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

// ApprovalsPage is the per-project queue of fresh agent enrollments
// awaiting an admin decision. Every fresh enroll lands here unless
// the redeemed install token had auto_approve=true. Operators sanity-
// check the reported hostname / IP / OS, optionally drop a reason
// note, and click Approve or Reject.
//
// Renders body-only — the parent FleetPage owns the page chrome
// (Fleet title, sub-tab strip, Enroll button) so this surface only
// adds a small toolbar (count + refresh) and the table.
//
// Reject is destructive (the agent's cert chain still validates, but
// the link gate refuses the next reconnect — to retry, the host has
// to be re-enrolled with a fresh install token), so we wrap it in an
// AlertDialog confirmation. Approve fires straight through; the worst
// case is approving something you shouldn't have, which the admin can
// undo by clicking Reject from Fleet on the host detail.
export default function ApprovalsPage() {
    const project = useCurrentProject();
    const qc = useQueryClient();

    const { data, isFetching, refetch } = useQuery({
        queryKey: qk.pendingHosts(project.id),
        queryFn: () => listPendingApprovals(project.id),
        // Live polling so a host that just enrolled appears within a
        // few seconds without the admin having to click Refresh.
        refetchInterval: 5000,
        refetchIntervalInBackground: false,
    });
    const hosts = data ?? [];

    function invalidatePending() {
        void qc.invalidateQueries({ queryKey: qk.pendingHosts(project.id) });
        void qc.invalidateQueries({ queryKey: qk.pendingHostsCount(project.id) });
        void qc.invalidateQueries({ queryKey: qk.hosts(project.id) });
    }

    const approveMu = useMutation({
        mutationFn: ({ hid, reason }: { hid: string; reason: string }) =>
            approveHost(project.id, hid, reason),
        onSuccess: () => {
            toast.success("Host approved");
            invalidatePending();
        },
        onError: (e) => toast.error(humanizeError(e)),
    });
    const rejectMu = useMutation({
        mutationFn: ({ hid, reason }: { hid: string; reason: string }) =>
            rejectHost(project.id, hid, reason),
        onSuccess: () => {
            toast.success("Host rejected");
            invalidatePending();
        },
        onError: (e) => toast.error(humanizeError(e)),
    });

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
                <span>
                    {hosts.length} pending · auto-expire 24 h
                </span>
                <RefreshButton
                    onClick={() => void refetch()}
                    loading={isFetching}
                    aria-label="Refresh pending approvals"
                />
            </div>
            <div style={{ flex: 1, minHeight: 0, overflow: "auto", padding: space[4] }}>
                {hosts.length === 0 ? (
                    <EmptyState
                        icon={<CheckCircle2 />}
                        title="No pending hosts"
                        description="Fresh enrollments land here. New agents that try to open a link
                            before approval are turned away with HTTP 425; they retry on a
                            backoff and connect automatically once you Approve."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>Hostname</TableHead>
                                <TableHead>OS</TableHead>
                                <TableHead>Primary IP</TableHead>
                                <TableHead>Egress</TableHead>
                                <TableHead>First seen</TableHead>
                                <TableHead>Fingerprint</TableHead>
                                <TableHead style={{ textAlign: "right" }}>Decision</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {hosts.map((h) => (
                                <ApprovalRow
                                    key={h.id}
                                    host={h}
                                    approving={approveMu.isPending}
                                    rejecting={rejectMu.isPending}
                                    onApprove={(reason) =>
                                        approveMu.mutate({ hid: h.id, reason })
                                    }
                                    onReject={(reason) =>
                                        rejectMu.mutate({ hid: h.id, reason })
                                    }
                                />
                            ))}
                        </TableBody>
                    </Table>
                )}
            </div>
        </div>
    );
}

function ApprovalRow({
    host,
    approving,
    rejecting,
    onApprove,
    onReject,
}: {
    host: Host;
    approving: boolean;
    rejecting: boolean;
    onApprove: (reason: string) => void;
    onReject: (reason: string) => void;
}) {
    const [reason, setReason] = useState("");
    const [confirmReject, setConfirmReject] = useState(false);

    return (
        <TableRow>
            <TableCell style={{ fontWeight: 500 }}>
                {host.hostname || <span style={{ color: palette.textMuted }}>—</span>}
            </TableCell>
            <TableCell>
                {host.os || <span style={{ color: palette.textMuted }}>—</span>}
            </TableCell>
            <TableCell>
                {host.primary_ip ? (
                    <RemoteAddr addr={host.primary_ip} info={host.primary_ip_info} />
                ) : (
                    <span style={{ color: palette.textMuted }}>—</span>
                )}
            </TableCell>
            <TableCell>
                {host.egress_ip ? (
                    <RemoteAddr addr={host.egress_ip} info={host.egress_ip_info} />
                ) : (
                    <span style={{ color: palette.textMuted }}>—</span>
                )}
            </TableCell>
            <TableCell>
                <span style={{ color: palette.textMuted }}>
                    {new Date(host.first_seen_at).toLocaleString()}
                </span>
            </TableCell>
            <TableCell>
                <Mono style={{ fontSize: 11 }}>
                    {host.fingerprint.slice(0, 16)}…
                </Mono>
                {host.fingerprint_fallback && (
                    <span
                        title="Agent didn't report a stable platform machine_id; identity is hostname + sorted MACs"
                        style={{
                            marginLeft: 6,
                            padding: "1px 5px",
                            borderRadius: 3,
                            fontSize: 10,
                            background: palette.surface,
                            border: `1px solid ${palette.border}`,
                            color: palette.textMuted,
                        }}
                    >
                        fallback
                    </span>
                )}
            </TableCell>
            <TableCell>
                <div
                    style={{
                        display: "flex",
                        gap: space[2],
                        alignItems: "center",
                        justifyContent: "flex-end",
                    }}
                >
                    <Input
                        placeholder="Reason (optional)"
                        value={reason}
                        onChange={(e) => setReason(e.target.value)}
                        style={{ maxWidth: 220 }}
                    />
                    <Button
                        type="button"
                        variant="default"
                        size="sm"
                        disabled={approving || rejecting}
                        onClick={() => onApprove(reason.trim())}
                        title="Allow this host to open a link and run RPCs"
                    >
                        {approving ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <CheckCircle2 className="size-3.5" />
                        )}
                        Approve
                    </Button>
                    <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        disabled={approving || rejecting}
                        onClick={() => setConfirmReject(true)}
                        title="Refuse this enrollment; the host must re-enroll with a fresh install token to try again"
                    >
                        {rejecting ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <XCircle className="size-3.5" />
                        )}
                        Reject
                    </Button>
                </div>
            </TableCell>

            <AlertDialog open={confirmReject} onOpenChange={setConfirmReject}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            <ShieldX
                                className="size-4 inline-block mr-2"
                                style={{ color: palette.danger }}
                            />
                            Reject {host.hostname || "this host"}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            The agent's cert chain stays valid, but the link gate
                            will refuse the next reconnect. To retry, the operator
                            of <Mono>{host.hostname || host.fingerprint.slice(0, 16)}</Mono>{" "}
                            must re-enroll with a fresh install token.
                            {reason ? (
                                <>
                                    <br />
                                    <br />
                                    Recorded reason: <em>{reason}</em>
                                </>
                            ) : null}
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                setConfirmReject(false);
                                onReject(reason.trim());
                            }}
                        >
                            Reject host
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </TableRow>
    );
}
