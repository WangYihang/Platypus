import { useState } from "react";
import { Hourglass, Loader2, Trash2 } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import Mono from "../../../components/Mono";
import StatusPill from "../../../components/StatusPill";
import { palette, radius, space } from "../../../layout/theme";
import { InstallArtifactListItem, revokeInstallArtifact } from "../../../lib/api";
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

interface Props {
    projectID: string;
    artifact: InstallArtifactListItem;
    invalidationKey: readonly unknown[];
}

// PendingArtifactCard renders an issued-but-not-yet-consumed install
// artifact as a placeholder tile in the Fleet card grid. Visually
// it matches the standard HostCard's footprint so the grid lays out
// evenly, but the body is a "waiting for agent" message instead of
// hardware data — there *is* no hardware data yet.
//
// Once the agent dials back, the install artifact transitions to
// `consumed` (the API filters it out of `listInstallArtifacts(_,
// false)`) and a real Host appears. The grid auto-replaces the
// placeholder with a HostCard — no special transition logic needed
// because both lists are re-rendered on every poll cycle.
export default function PendingArtifactCard({
    projectID,
    artifact,
    invalidationKey,
}: Props) {
    const qc = useQueryClient();
    const [confirmRevoke, setConfirmRevoke] = useState(false);

    const revokeMu = useMutation({
        mutationFn: () => revokeInstallArtifact(projectID, artifact.download_id),
        onSuccess: () => {
            toast.success("Install link revoked");
            void qc.invalidateQueries({ queryKey: invalidationKey });
        },
        onError: (e) => toast.error(`Couldn't revoke: ${humanizeError(e)}`),
    });

    const targetLabel =
        artifact.target_os || artifact.target_arch
            ? `${artifact.target_os || "any"}/${artifact.target_arch || "any"}`
            : "auto-detect";

    return (
        <div
            data-testid="fleet-pending-artifact"
            data-download-id={artifact.download_id}
            style={{
                background: palette.surface,
                border: `1px dashed ${palette.warning}`,
                borderRadius: radius.md,
                padding: `${space[4]}px ${space[4]}px ${space[3]}px`,
                display: "flex",
                flexDirection: "column",
                gap: space[3],
                color: palette.textPrimary,
                fontFamily: "var(--font-geist-mono)",
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    minWidth: 0,
                }}
            >
                <Hourglass
                    className="size-4 animate-pulse"
                    style={{ color: palette.warning, flexShrink: 0 }}
                />
                <span
                    style={{
                        fontWeight: 600,
                        fontSize: 14,
                        flex: 1,
                        minWidth: 0,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                    }}
                    title={artifact.pat_description || artifact.download_id}
                >
                    {artifact.pat_description || "Waiting for agent…"}
                </span>
                <StatusPill tone="warning">pending</StatusPill>
            </div>

            <div style={{ fontSize: 12, color: palette.textSecondary, lineHeight: 1.5 }}>
                <span style={{ color: palette.textMuted }}>
                    Install command issued — waiting for the agent to dial back.
                </span>
            </div>

            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "auto 1fr",
                    rowGap: 4,
                    columnGap: space[2],
                    fontSize: 12,
                    color: palette.textSecondary,
                }}
            >
                <span style={{ color: palette.textMuted }}>Target</span>
                <span>
                    <Mono>{targetLabel}</Mono>
                </span>
                <span style={{ color: palette.textMuted }}>Server</span>
                <span
                    style={{
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                    title={artifact.server_endpoint}
                >
                    <Mono>{artifact.server_endpoint}</Mono>
                </span>
                <span style={{ color: palette.textMuted }}>Expires</span>
                <span>{fromNow(artifact.expires_at)}</span>
            </div>

            <div
                style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    fontSize: 11,
                    color: palette.textMuted,
                    borderTop: `1px solid ${palette.border}`,
                    paddingTop: space[2],
                }}
            >
                <Mono size={11}>{artifact.download_id}</Mono>
                <Button
                    variant="ghost"
                    size="sm"
                    className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                    onClick={() => setConfirmRevoke(true)}
                    disabled={revokeMu.isPending}
                    data-testid="fleet-pending-artifact-revoke"
                    title="Revoke this install command"
                >
                    {revokeMu.isPending ? (
                        <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                        <Trash2 className="size-3.5" />
                    )}
                </Button>
            </div>

            <AlertDialog open={confirmRevoke} onOpenChange={setConfirmRevoke}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke install link?</AlertDialogTitle>
                        <AlertDialogDescription>
                            The curl command will stop working immediately. The host
                            it was issued for has to be re-enrolled with a fresh
                            install token to try again.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                setConfirmRevoke(false);
                                revokeMu.mutate();
                            }}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Revoke
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
}
