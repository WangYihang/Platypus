// Services tab — sys-services-darwin / -windows. Different from the
// existing sys-systemd-linux Services tab because launchd / SCM
// expose smaller RPC machinery (no show_unit detail; simpler
// status / startType fields). Wire shape across darwin and
// windows is similar enough to share one component.

import RPCTable, { type Column, type RowAction } from "../shared/RPCTable";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Unit {
    name?: string; // Windows
    label?: string; // Darwin
    displayName?: string;
    status?: string;
    startType?: string;
    pid?: number;
    active?: string;
}

interface ListUnitsResponse {
    units?: Unit[];
    error?: string;
}

const STATE_COLOR: Record<string, string> = {
    running: palette.success,
    active: palette.success,
    stopped: palette.textMuted,
    inactive: palette.textMuted,
    failed: palette.danger,
    paused: palette.warning,
};

function StateBadge({ state }: { state: string }) {
    const color = STATE_COLOR[state] ?? palette.textMuted;
    return (
        <span
            style={{
                display: "inline-block",
                padding: "2px 8px",
                borderRadius: 999,
                fontSize: 11,
                fontWeight: 500,
                color: "#fff",
                background: color,
            }}
        >
            {state}
        </span>
    );
}

const COLUMNS: ReadonlyArray<Column<Unit>> = [
    {
        field: "name",
        label: "Service",
        primary: true,
        render: (row) => row.name ?? row.label ?? "—",
    },
    {
        field: "displayName",
        label: "Display",
        truncate: true,
        render: (row) => row.displayName ?? "—",
    },
    {
        field: "status",
        label: "Status",
        render: (row) => {
            const s = row.status ?? row.active ?? "unknown";
            return <StateBadge state={s} />;
        },
    },
    {
        field: "startType",
        label: "Start type",
        render: (row) => row.startType ?? "—",
    },
];

const ACTIONS: ReadonlyArray<RowAction<Unit>> = [
    {
        id: "start",
        label: "Start",
        method: "unit_action",
        args: (u) => ({ name: u.name ?? u.label ?? "", action: "start" }),
    },
    {
        id: "stop",
        label: "Stop",
        method: "unit_action",
        args: (u) => ({ name: u.name ?? u.label ?? "", action: "stop" }),
        confirm: (u) => `Stop ${u.name ?? u.label}?`,
        danger: true,
    },
    {
        id: "restart",
        label: "Restart",
        method: "unit_action",
        args: (u) => ({ name: u.name ?? u.label ?? "", action: "restart" }),
        confirm: (u) => `Restart ${u.name ?? u.label}?`,
    },
];

export function Services({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    return (
        <RPCTable<ListUnitsResponse, Unit>
            projectID={projectID}
            agentID={agentID}
            pluginID={pluginID}
            method="list_units"
            active={active}
            requestForm={[
                {
                    field: "filter",
                    kind: "search",
                    label: "Search",
                    default: "",
                    debounceMs: 200,
                },
            ]}
            buildRequest={(form) =>
                form.filter ? { filter: form.filter } : {}
            }
            rowsFrom={(r) => r.units ?? []}
            rowKey={(u) => u.name ?? u.label ?? ""}
            columns={COLUMNS}
            actions={ACTIONS}
            refreshMs={30000}
            emptyText="No services match."
        />
    );
}
