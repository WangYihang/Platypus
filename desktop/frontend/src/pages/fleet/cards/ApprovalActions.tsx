import { useState } from "react";
import { CheckCircle2, Loader2, ShieldX, XCircle } from "lucide-react";

import Mono from "../../../components/Mono";
import { palette, space } from "../../../layout/theme";
import { Host } from "../../../lib/api";
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

interface Props {
    host: Host;
    approving: boolean;
    rejecting: boolean;
    onApprove: () => void;
    onReject: () => void;
}

// ApprovalActions is the inline Approve / Reject button pair that
// renders inside a HostCard when `host.approval_status === "pending"`.
// Approve fires straight through (worst case = revoke later).
// Reject is wrapped in an AlertDialog because it forces a re-enroll
// — same destructive-action pattern as the dedicated ApprovalsPage,
// just inlined into the card for a one-click flow that doesn't
// require a page change.
export default function ApprovalActions({
    host,
    approving,
    rejecting,
    onApprove,
    onReject,
}: Props) {
    const [confirmReject, setConfirmReject] = useState(false);

    return (
        <div
            data-testid="host-card-approval-actions"
            onClick={(e) => {
                // The HostCard wrapper is itself a button — stop the
                // click so approving doesn't also navigate into the
                // host detail view.
                e.stopPropagation();
            }}
            style={{
                display: "flex",
                gap: space[2],
                marginTop: space[2],
                paddingTop: space[2],
                borderTop: `1px dashed ${palette.warning}`,
            }}
        >
            <Button
                type="button"
                size="sm"
                disabled={approving || rejecting}
                onClick={onApprove}
                title="Allow this host to open a link and run RPCs"
                data-testid="host-card-approve"
                style={{ flex: 1 }}
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
                data-testid="host-card-reject"
                style={{ flex: 1 }}
            >
                {rejecting ? (
                    <Loader2 className="size-3.5 animate-spin" />
                ) : (
                    <XCircle className="size-3.5" />
                )}
                Reject
            </Button>

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
                            The agent's cert chain stays valid, but the link gate will
                            refuse the next reconnect. To retry, the operator of{" "}
                            <Mono>{host.hostname || host.fingerprint.slice(0, 16)}</Mono>{" "}
                            must re-enroll with a fresh install token.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                setConfirmReject(false);
                                onReject();
                            }}
                        >
                            Reject host
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
}
