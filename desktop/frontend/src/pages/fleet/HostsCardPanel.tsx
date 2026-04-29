import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";

import EmptyState from "../../components/EmptyState";
import FilterToolbar from "../../components/FilterToolbar";
import { useCurrentProject } from "../../layout/ProjectShell";
import { space } from "../../layout/theme";
import { listHosts, listInstallArtifacts } from "../../lib/api";
import { qk } from "../../lib/queryKeys";
import { humanizeError } from "../../lib/humanizeError";
import { isOnline } from "../../lib/time";

import CardGridSkeleton from "./cards/CardGridSkeleton";
import EnrollAgentTile from "./cards/EnrollAgentTile";
import HostCard from "./cards/HostCard";
import PendingArtifactCard from "./cards/PendingArtifactCard";
import { useApprovalMutations } from "./cards/useApprovalMutations";

// HostsCardPanel renders the Fleet inventory as a responsive grid of
// cards. After the 2026-04 enrollment IA pass, the grid composes
// three card kinds (in this order):
//
//   1. EnrollAgentTile      — always-on "+" entry to the wizard,
//                              first tile so an empty fleet still
//                              shows an action card.
//   2. PendingArtifactCard  — install command issued but the agent
//                              hasn't dialled back yet. Shows a
//                              countdown + revoke control.
//   3. HostCard             — real host. When approval_status is
//                              "pending", the card carries inline
//                              Approve / Reject buttons (no need to
//                              jump to /fleet/approvals for the
//                              common single-host case).
//
// The two underlying queries (hosts, install artifacts) poll on a
// short interval so the visual transition `pending placeholder` →
// `pending host with approve buttons` → `approved host` happens
// without manual refresh.
const POLL_INTERVAL_MS = 4000;

export default function HostsCardPanel() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const queryClient = useQueryClient();
    const [query, setQuery] = useState("");

    const hostsQuery = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
        refetchInterval: POLL_INTERVAL_MS,
    });
    const artifactsKey = ["installArtifactsActive", project.id] as const;
    const artifactsQuery = useQuery({
        queryKey: artifactsKey,
        queryFn: () => listInstallArtifacts(project.id, false),
        refetchInterval: POLL_INTERVAL_MS,
    });

    const hosts = hostsQuery.data ?? null;
    const artifacts = artifactsQuery.data ?? [];

    function refresh() {
        queryClient.invalidateQueries({ queryKey: qk.hosts(project.id) });
        queryClient.invalidateQueries({ queryKey: artifactsKey });
    }

    useEffect(() => {
        if (hostsQuery.error) {
            toast.error(`load hosts: ${humanizeError(hostsQuery.error)}`);
        }
    }, [hostsQuery.error]);

    const filteredHosts = useMemo(() => {
        if (!hosts) return null;
        const q = query.trim().toLowerCase();
        if (!q) return hosts;
        return hosts.filter((h) =>
            [h.hostname, h.primary_alias, h.os, h.machine_id, h.primary_ip]
                .filter(Boolean)
                .some((v) => String(v).toLowerCase().includes(q)),
        );
    }, [hosts, query]);

    // Pending placeholders only show when the operator hasn't typed
    // a search. They have no hostname / IP / OS to match against; if
    // the operator is searching, hiding them keeps the result set
    // honest.
    const pendingArtifacts = useMemo(() => {
        if (query.trim()) return [];
        return artifacts.filter(
            (a) => a.status === "pending" && !a.consumed_at,
        );
    }, [artifacts, query]);

    const onlineCount = hosts?.filter((h) => isOnline(h.last_seen_at)).length ?? 0;
    const loading = hostsQuery.isFetching || artifactsQuery.isFetching;

    const { approveMu, rejectMu } = useApprovalMutations(project.id);

    const noResults =
        hosts !== null &&
        filteredHosts !== null &&
        filteredHosts.length === 0 &&
        pendingArtifacts.length === 0 &&
        query.trim().length > 0;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <FilterToolbar
                search={{
                    value: query,
                    onChange: setQuery,
                    placeholder: "Search hostname, alias, OS, IP",
                    minWidth: 280,
                }}
                count={
                    hosts === null
                        ? "Loading…"
                        : `${hosts.length} total · ${onlineCount} online`
                }
                refreshLoading={loading}
                onRefresh={refresh}
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {hosts === null ? (
                    <CardGridSkeleton />
                ) : noResults ? (
                    <EmptyState
                        title="No matches"
                        description={`No host or pending enrollment matches "${query}".`}
                    />
                ) : (
                    <div
                        data-testid="fleet-cards-grid"
                        style={{
                            display: "grid",
                            gridTemplateColumns:
                                "repeat(auto-fill, minmax(280px, 1fr))",
                            gap: space[3],
                        }}
                    >
                        <EnrollAgentTile />
                        {pendingArtifacts.map((a) => (
                            <PendingArtifactCard
                                key={a.download_id}
                                projectID={project.id}
                                artifact={a}
                                invalidationKey={artifactsKey}
                            />
                        ))}
                        {(filteredHosts ?? []).map((h) => (
                            <HostCard
                                key={h.id}
                                host={h}
                                approving={
                                    approveMu.isPending &&
                                    approveMu.variables?.hid === h.id
                                }
                                rejecting={
                                    rejectMu.isPending &&
                                    rejectMu.variables?.hid === h.id
                                }
                                onApprove={() =>
                                    approveMu.mutate({ hid: h.id, reason: "" })
                                }
                                onReject={() =>
                                    rejectMu.mutate({ hid: h.id, reason: "" })
                                }
                                onOpen={() =>
                                    navigate(
                                        `/projects/${project.slug}/hosts/${h.id}/files`,
                                    )
                                }
                            />
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}
