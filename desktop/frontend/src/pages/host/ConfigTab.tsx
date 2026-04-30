import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";

import { Button } from "@/components/ui/button";

import EmptyState from "../../components/EmptyState";
import { palette, radius, space } from "../../layout/theme";
import {
    AuditorResult,
    AvailableAuditor,
    HostConfigAudit,
    getHostConfigAudit,
    listAvailableAuditors,
    reauditHost,
} from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";

import CoveragePanel from "./config/CoveragePanel";
import Header from "./config/Header";
import LeaksList from "./config/LeaksList";

interface Props {
    projectID: string;
    hostID: string;
    active: boolean;
}

// ConfigTab is the host-level entrypoint for the sensitive-information
// audit. It wires three data sources together:
//
//   1. listAvailableAuditors() — every registered auditor on the
//      agent, used to render the coverage panel even when no audit
//      has run. Returns null when the agent is offline; the UI then
//      falls back to deriving the auditor list from the persisted
//      audit's auditors[] array.
//   2. getHostConfigAudit() — the latest persisted audit + leaks.
//      Returns null when the host has never been audited; the UI
//      shows the placeholder empty state with a single "Run audit"
//      button in that case.
//   3. reauditHost() mutation — POSTs the rescan; partial re-audits
//      pass auditor_ids so a per-row "Run" click only re-evaluates
//      that one auditor (the server merges with the prior audit).
//
// Both queries are gated on `active` so a hidden tab does no work.
export function ConfigTab({ projectID, hostID, active }: Props) {
    const queryClient = useQueryClient();

    const auditorsQuery = useQuery({
        queryKey: qk.hostConfigAuditors(projectID, hostID),
        queryFn: () => listAvailableAuditors(projectID, hostID),
        enabled: active,
        refetchOnWindowFocus: false,
        // The agent may genuinely be offline — null is a valid result
        // shape, not an error to retry forever on.
        retry: false,
    });

    const auditQuery = useQuery({
        queryKey: qk.hostConfigAudit(projectID, hostID),
        queryFn: () => getHostConfigAudit(projectID, hostID),
        enabled: active,
        refetchOnWindowFocus: false,
    });

    // Track which auditor ids are mid-rerun so the per-row spinner
    // lights up just on those rows. An empty set means "no partial
    // active"; we use the mutation's isPending flag to know whether
    // a full re-audit is running.
    const [runningSet, setRunningSet] = useState<Set<string>>(new Set());

    const reaudit = useMutation({
        mutationFn: (vars: { auditor_ids?: string[] }) =>
            reauditHost(projectID, hostID, vars),
        onMutate: (vars) => {
            setRunningSet(vars.auditor_ids ? new Set(vars.auditor_ids) : new Set());
        },
        onSettled: () => {
            setRunningSet(new Set());
        },
        onSuccess: (fresh) => {
            queryClient.setQueryData(qk.hostConfigAudit(projectID, hostID), fresh);
            queryClient.invalidateQueries({
                queryKey: qk.hostConfigAudits(projectID, hostID, 10),
            });
            queryClient.invalidateQueries({ queryKey: qk.hosts(projectID) });
        },
    });

    const audit = auditQuery.data;
    const auditors = useDerivedAuditors(auditorsQuery.data, audit);

    // Map of auditor id -> last AuditorResult, used by CoveragePanel
    // for the per-row status badge.
    const lastResults = useMemo(() => {
        const map = new Map<string, AuditorResult>();
        if (audit) {
            for (const a of audit.auditors ?? []) {
                map.set(a.id, a);
            }
        }
        return map;
    }, [audit]);

    const isAnyRunning = reaudit.isPending;
    const hasNeverAudited =
        !audit && !auditQuery.isLoading && !auditQuery.isFetching;

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <Header
                audit={audit}
                isAnyRunning={isAnyRunning}
                onReauditAll={() => reaudit.mutate({})}
            />

            {auditQuery.error && (
                <Alert kind="danger">{humanizeError(auditQuery.error)}</Alert>
            )}
            {reaudit.error && (
                <Alert kind="warning">
                    Audit failed — {humanizeError(reaudit.error)}
                </Alert>
            )}

            {hasNeverAudited && auditors.length === 0 && (
                <EmptyState
                    icon={<KeyRound />}
                    title="This host has not been audited yet"
                    description="Run a configuration audit to scan for credentials in environment variables, shell history, and known config files."
                    action={
                        <Button
                            type="button"
                            size="sm"
                            onClick={() => reaudit.mutate({})}
                            disabled={isAnyRunning}
                        >
                            {isAnyRunning ? "Auditing…" : "Run first audit"}
                        </Button>
                    }
                />
            )}

            {auditors.length > 0 && (
                <CoveragePanel
                    auditors={auditors}
                    lastResults={lastResults}
                    runningSet={runningSet}
                    isAnyRunning={isAnyRunning}
                    onRerun={(id) => reaudit.mutate({ auditor_ids: [id] })}
                />
            )}

            {audit && <LeaksList leaks={audit.leaks ?? []} />}
        </div>
    );
}

// useDerivedAuditors prefers the live agent's auditor list; if the
// agent is offline (auditorsQuery returned null), falls back to
// synthesising minimal AvailableAuditor entries from the persisted
// audit's auditors[] array so the coverage panel still renders.
function useDerivedAuditors(
    live: AvailableAuditor[] | null | undefined,
    audit: HostConfigAudit | null | undefined,
): AvailableAuditor[] {
    return useMemo(() => {
        if (live && live.length > 0) return live;
        if (!audit?.auditors) return [];
        return audit.auditors.map((a) => ({
            id: a.id,
            category: a.category,
            applicable: a.status !== "skipped",
        }));
    }, [live, audit]);
}

function Alert({
    kind,
    children,
}: {
    kind: "danger" | "warning";
    children: React.ReactNode;
}) {
    const fg = kind === "danger" ? palette.danger : palette.warning;
    const bg =
        kind === "danger"
            ? "rgba(238, 0, 0, 0.08)"
            : "rgba(245, 166, 35, 0.08)";
    return (
        <div
            role="alert"
            style={{
                fontSize: 12,
                color: fg,
                background: bg,
                border: `1px solid ${fg}`,
                borderRadius: radius.sm,
                padding: `${space[2]}px ${space[3]}px`,
            }}
        >
            {children}
        </div>
    );
}

export default ConfigTab;
