import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
    AgentUpgradeRequest,
    AgentUpgradeResponse,
    triggerAgentUpgrade,
} from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";

// useAgentUpgradeMutation centralises the trigger-upgrade mutation.
// Lifted out of HostView so the toast vocabulary + invalidation list
// stays in one place — drift here would silently leave the host's
// build-version row stale after a successful upgrade.
//
// On success we invalidate hosts() so the list view refreshes any
// "outdated" pills, plus host(projectID, hostID) for the detail
// view's build-version row. We don't invalidate hostSysInfo: the
// agent will reconnect and the next heartbeat will refresh it
// naturally, and a stale-while-the-agent-restarts read shows the
// pre-upgrade values harmlessly.
export function useAgentUpgradeMutation(projectID: string, hostID: string) {
    const qc = useQueryClient();
    return useMutation<AgentUpgradeResponse, Error, AgentUpgradeRequest>({
        mutationFn: (body: AgentUpgradeRequest) =>
            triggerAgentUpgrade(projectID, hostID, body),
        // Status-code policy on the server: 200 / 202 for every case
        // where the upgrade flow actually ran, 5xx only when the
        // request never reached the agent. So onSuccess fires for
        // both "exited" (agent installing) and "failed" (agent
        // reported a problem) — both are useful operator feedback.
        // onError covers the genuine RPC errors (link drop, auth
        // expired, agent not connected → 404).
        onSuccess: (resp) => {
            if (resp.status === "exited") {
                toast.success(
                    resp.resolved_version
                        ? `Agent upgrading to ${resp.resolved_version}; supervisor will restart it shortly`
                        : "Agent upgrading; supervisor will restart it shortly",
                );
            } else if (resp.status === "in_progress") {
                toast.info(
                    "Upgrade is taking longer than expected; check the activity log for the final outcome",
                );
            } else if (resp.status === "failed") {
                toast.error(
                    `Upgrade failed: ${resp.error_code || "unknown"}` +
                        (resp.error_message ? ` — ${resp.error_message}` : ""),
                );
            } else {
                toast.warning(`Upgrade ended in unexpected state: ${resp.status}`);
            }
            void qc.invalidateQueries({ queryKey: qk.hosts(projectID) });
            void qc.invalidateQueries({ queryKey: qk.host(projectID, hostID) });
        },
        onError: (e) => toast.error(humanizeError(e)),
    });
}
