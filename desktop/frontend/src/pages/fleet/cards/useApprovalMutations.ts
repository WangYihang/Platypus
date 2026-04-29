import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { approveHost, rejectHost } from "../../../lib/api";
import { humanizeError } from "../../../lib/humanizeError";
import { qk } from "../../../lib/queryKeys";

// useApprovalMutations centralises the approve / reject mutations
// HostCard's inline action buttons need. Lifted out of the card so
// the component file stays tight and so the mutation invalidation
// list (hosts, pending hosts, pending count) is defined once — drift
// here would silently leave the approvals badge stale after an
// inline approval.
export function useApprovalMutations(projectID: string) {
    const qc = useQueryClient();

    function invalidate() {
        void qc.invalidateQueries({ queryKey: qk.hosts(projectID) });
        void qc.invalidateQueries({ queryKey: qk.pendingHosts(projectID) });
        void qc.invalidateQueries({ queryKey: qk.pendingHostsCount(projectID) });
    }

    const approveMu = useMutation({
        mutationFn: ({ hid, reason }: { hid: string; reason: string }) =>
            approveHost(projectID, hid, reason),
        onSuccess: () => {
            toast.success("Host approved");
            invalidate();
        },
        onError: (e) => toast.error(humanizeError(e)),
    });

    const rejectMu = useMutation({
        mutationFn: ({ hid, reason }: { hid: string; reason: string }) =>
            rejectHost(projectID, hid, reason),
        onSuccess: () => {
            toast.success("Host rejected");
            invalidate();
        },
        onError: (e) => toast.error(humanizeError(e)),
    });

    return { approveMu, rejectMu };
}
