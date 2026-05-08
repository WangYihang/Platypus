// Cron tab — sys-cron-linux. Single RPC `list_cron_jobs` returns
// the union of /etc/crontab + /etc/cron.d + /var/spool/cron/* +
// run-parts directories + /etc/anacrontab. Each row carries `kind`
// to disambiguate the source. Disabled (commented-out) lines are
// hidden by default; a toggle surfaces them for forensic audits.

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface CronJob {
    source?: string;
    user?: string;
    schedule?: string;
    command?: string;
    kind?: string;
    lineNo?: number;
    enabled?: boolean;
}

interface ListCronJobsResponse {
    jobs?: CronJob[];
    error?: string;
}

const KIND_LABEL: Record<string, string> = {
    crontab: "user",
    system_crontab: "system",
    cron_d: "cron.d",
    run_parts: "run-parts",
    anacron: "anacron",
};

const COLUMNS: ReadonlyArray<Column<CronJob>> = [
    {
        field: "schedule",
        label: "Schedule",
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {row.schedule || "—"}
            </span>
        ),
    },
    {
        field: "user",
        label: "User",
        render: (row) => row.user || "—",
    },
    {
        field: "command",
        label: "Command",
        primary: true,
        truncate: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {row.command || "—"}
            </span>
        ),
    },
    {
        field: "kind",
        label: "Kind",
        render: (row) => KIND_LABEL[row.kind ?? ""] ?? row.kind ?? "—",
    },
    {
        field: "source",
        label: "Source",
        truncate: true,
        render: (row) => row.source || "—",
    },
    {
        field: "enabled",
        label: "Enabled",
        render: (row) =>
            row.enabled === false ? (
                <span style={{ color: palette.textMuted }}>off</span>
            ) : (
                <span style={{ color: palette.success }}>on</span>
            ),
    },
];

export function CronJobs({
    projectID,
    agentID,
    active,
}: PluginUIProps) {
    return (
        <RPCTable<ListCronJobsResponse, CronJob>
            projectID={projectID}
            agentID={agentID}
            pluginID="com.platypus.sys-cron-linux"
            method="list_cron_jobs"
            active={active}
            requestForm={[
                {
                    field: "include_disabled",
                    kind: "toggle",
                    label: "Include disabled / commented-out",
                    default: false,
                },
            ]}
            buildRequest={(form) => ({
                include_disabled: Boolean(form.include_disabled),
            })}
            rowsFrom={(r) => r.jobs ?? []}
            rowKey={(j, idx) =>
                `${j.source ?? ""}-${j.lineNo ?? 0}-${j.command ?? ""}-${idx}`
            }
            columns={COLUMNS}
            refreshMs={0}
            emptyText="No cron jobs found."
        />
    );
}
