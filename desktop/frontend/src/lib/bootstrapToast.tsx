// Shared post-bootstrap "Admin created" toast. The cleanup advice
// (delete <data-dir>/bootstrap.secret on the server) is the security-
// relevant part: leaving the file behind exposes the next bootstrap
// route to anyone with read access to the data dir or its backups.
//
// We render it sticky (no auto-dismiss) with an explicit "Got it"
// dismiss action — the previous 8s auto-dismiss meant operators
// reading the wizard's last step would miss it entirely. Centralising
// the wording so Onboarding / Login / AddServerDialog can't drift.

import { Trans } from "react-i18next";
import { toast } from "sonner";

import Mono from "../components/Mono";
import i18n from "../i18n";

export function showAdminCreatedToast(): void {
    toast.success(i18n.t("onboarding:adminCreated.title"), {
        description: (
            <Trans
                ns="onboarding"
                i18nKey="adminCreated.description"
                components={{ mono: <Mono /> }}
            />
        ),
        duration: Infinity,
        action: {
            label: i18n.t("common:actions.gotIt"),
            onClick: () => {
                /* sonner closes the toast on action click */
            },
        },
    });
}
