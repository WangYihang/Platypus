// Scheduled Tasks tab — sys-tasks-windows. Single RPC `list_tasks`
// shells Get-ScheduledTask + Get-ScheduledTaskInfo and returns each
// task with state, triggers, actions, last/next run times, and the
// principal it runs as. Sibling of sys-cron-linux for the Windows
// side of "what's scheduled to fire on this host".

import RPCTable, { type Column } from "../shared/RPCTable";
import { Badge, type BadgeTone } from "../shared/Badge";
import type { PluginUIProps } from "../registry";

interface ScheduledTaskAction {
    execute?: string;
    arguments?: string;
    workingDir?: string;
}

interface ScheduledTask {
    taskName?: string;
    taskPath?: string;
    state?: string;
    author?: string;
    description?: string;
    triggers?: string[];
    actions?: ScheduledTaskAction[];
    lastRunUnix?: number;
    lastResult?: number;
    nextRunUnix?: number;
    runAsUser?: string;
}

interface ListTasksResponse {
    tasks?: ScheduledTask[];
    error?: string;
    totalCount?: number;
    hasMore?: boolean;
}

const STATE_TONE: Record<string, BadgeTone> = {
    Ready: "success",
    Running: "success",
    Disabled: "muted",
    Queued: "warning",
    Unknown: "muted",
};

function StateBadge({ state }: { state: string }) {
    return (
        <Badge tone={STATE_TONE[state] ?? "muted"} shape="tag">
            {state}
        </Badge>
    );
}

function formatUnix(t: number | undefined) {
    if (!t || t <= 0) return "—";
    return new Date(t * 1000).toLocaleString();
}

function summariseAction(actions: ScheduledTaskAction[] | undefined) {
    if (!actions || actions.length === 0) return "—";
    const first = actions[0]!;
    const exec = first.execute ?? "—";
    const args = first.arguments ? ` ${first.arguments}` : "";
    if (actions.length > 1) {
        return `${exec}${args}  (+${actions.length - 1} more)`;
    }
    return `${exec}${args}`;
}

const COLUMNS: ReadonlyArray<Column<ScheduledTask>> = [
    {
        field: "taskName",
        label: "Task",
        primary: true,
        render: (row) => row.taskName || "—",
    },
    {
        field: "taskPath",
        label: "Path",
        truncate: true,
        render: (row) => row.taskPath || "—",
    },
    {
        field: "state",
        label: "State",
        render: (row) => <StateBadge state={row.state ?? "Unknown"} />,
    },
    {
        field: "triggers",
        label: "Triggers",
        truncate: true,
        render: (row) =>
            row.triggers && row.triggers.length > 0
                ? row.triggers.join("; ")
                : "—",
    },
    {
        field: "actions",
        label: "Action",
        truncate: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {summariseAction(row.actions)}
            </span>
        ),
    },
    {
        field: "nextRunUnix",
        label: "Next run",
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 11 }}>
                {formatUnix(row.nextRunUnix)}
            </span>
        ),
    },
    {
        field: "lastRunUnix",
        label: "Last run",
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 11 }}>
                {formatUnix(row.lastRunUnix)}
            </span>
        ),
    },
    {
        field: "runAsUser",
        label: "Run as",
        render: (row) => row.runAsUser || "—",
    },
];

export function Tasks({ projectID, agentID, active }: PluginUIProps) {
    return (
        <RPCTable<ListTasksResponse, ScheduledTask>
            projectID={projectID}
            agentID={agentID}
            pluginID="com.platypus.sys-tasks-windows"
            method="list_tasks"
            active={active}
            requestForm={[
                {
                    field: "filter",
                    kind: "search",
                    label: "Search",
                    default: "",
                    debounceMs: 250,
                    placeholder: "Update / Defender",
                },
                {
                    field: "path_prefix",
                    kind: "search",
                    label: "Path prefix",
                    default: "",
                    debounceMs: 250,
                    placeholder: "\\Microsoft\\Windows",
                },
                {
                    field: "include_disabled",
                    kind: "toggle",
                    label: "Include disabled",
                    default: false,
                },
            ]}
            buildRequest={(form) => ({
                ...(form.filter ? { filter: form.filter } : {}),
                ...(form.path_prefix
                    ? { path_prefix: form.path_prefix }
                    : {}),
                include_disabled: Boolean(form.include_disabled),
            })}
            rowsFrom={(r) => r.tasks ?? []}
            rowKey={(t, idx) =>
                `${t.taskPath ?? ""}${t.taskName ?? ""}-${idx}`
            }
            columns={COLUMNS}
            refreshMs={0}
            emptyText="No scheduled tasks match."
            pagination={{ kind: "offset", pageSizeOptions: [50, 100, 200, 500] }}
        />
    );
}
