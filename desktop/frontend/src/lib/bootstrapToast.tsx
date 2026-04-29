// Shared post-bootstrap "Admin created" toast. The cleanup advice
// (delete <data-dir>/bootstrap.secret on the server) is the security-
// relevant part: leaving the file behind exposes the next bootstrap
// route to anyone with read access to the data dir or its backups.
//
// We render it sticky (no auto-dismiss) with an explicit "Got it"
// dismiss action — the previous 8s auto-dismiss meant operators
// reading the wizard's last step would miss it entirely. Centralising
// the wording so Onboarding / Login / AddServerDialog can't drift.

import { toast } from "sonner";

import Mono from "../components/Mono";

export function showAdminCreatedToast(): void {
    toast.success("Admin created — welcome to Platypus", {
        description: (
            <span>
                Delete <Mono>&lt;data-dir&gt;/bootstrap.secret</Mono> on the
                server to keep the secret out of backups. The server also
                clears it on the next boot once a user exists.
            </span>
        ),
        duration: Infinity,
        action: {
            label: "Got it",
            onClick: () => {
                /* sonner closes the toast on action click */
            },
        },
    });
}
