import { useState } from "react";

import { Button } from "@/components/ui/button";
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

import { useAgentUpgradeMutation } from "./useAgentUpgrade";

// UpgradeAgentButton fires a server-driven self-upgrade against this
// host. AlertDialog confirms before the mutation runs because the
// agent will exit + restart — destructive in the systemd-level sense
// even though the binary is rolled back on rename failure. The
// button hides itself when there's no manifest to upgrade *to*
// (distributor disabled), keeping the row uncluttered for self-
// hosted setups that don't ship releases.
//
// Lives outside pills.tsx (its sibling, holding BuildVersionValue +
// other purely-presentational helpers) because this component owns
// state — useAgentUpgradeMutation — and the dialog. Mixing stateful
// components into pills.tsx would erode that file's "no hooks, no
// state" invariant.
export function UpgradeAgentButton({
    projectID,
    hostID,
    currentVersion,
    latestVersion,
}: {
    projectID: string;
    hostID: string;
    currentVersion: string | undefined;
    latestVersion: string | undefined;
}) {
    const [open, setOpen] = useState(false);
    const upgradeMu = useAgentUpgradeMutation(projectID, hostID);

    if (!latestVersion) return null;
    const isCurrent = currentVersion === latestVersion;
    const label = upgradeMu.isPending
        ? "Upgrading…"
        : isCurrent
          ? "Reinstall"
          : "Upgrade";

    return (
        <>
            <Button
                size="sm"
                variant={isCurrent ? "outline" : "default"}
                onClick={() => setOpen(true)}
                disabled={upgradeMu.isPending}
                data-testid="host-upgrade-button"
            >
                {label}
            </Button>

            <AlertDialog open={open} onOpenChange={setOpen}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            {isCurrent
                                ? `Reinstall agent ${latestVersion}?`
                                : `Upgrade agent to ${latestVersion}?`}
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            The host's agent will fetch the signed manifest, verify the
                            Ed25519 signature, atomically replace its binary, and exit
                            with code 75 so the supervisor restarts it under the new
                            build.{" "}
                            {currentVersion
                                ? `Currently running ${currentVersion}.`
                                : "Current build version unknown."}
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel disabled={upgradeMu.isPending}>
                            Cancel
                        </AlertDialogCancel>
                        <AlertDialogAction
                            disabled={upgradeMu.isPending}
                            onClick={(e) => {
                                e.preventDefault(); // keep the dialog open while the request is in flight
                                upgradeMu.mutate(
                                    { target_version: latestVersion, channel: "stable" },
                                    {
                                        onSettled: () => setOpen(false),
                                    },
                                );
                            }}
                            data-testid="host-upgrade-confirm"
                        >
                            {upgradeMu.isPending ? "Upgrading…" : "Confirm"}
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}
