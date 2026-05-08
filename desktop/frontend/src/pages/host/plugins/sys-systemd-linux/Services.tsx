// sys-systemd-linux Services tab — the Sprint 3 pilot for the
// schema-less, TypeScript-typed plugin UI pattern. ~50 LoC of
// configuration over a generic <RPCTable>.

import RPCTable, { type Column, type RowAction } from "../shared/RPCTable";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Unit {
    name: string;
    load: string;
    active: string;
    sub: string;
    description: string;
}

interface ListUnitsResponse {
    units?: Unit[];
    error?: string;
    totalCount?: number;
    hasMore?: boolean;
}

const PLUGIN_ID = "com.platypus.sys-systemd-linux";

const STATE_COLOR: Record<string, string> = {
    active: palette.success,
    inactive: palette.textMuted,
    failed: palette.danger,
    activating: palette.warning,
    deactivating: palette.warning,
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
    { field: "name", label: "Service", primary: true },
    {
        field: "active",
        label: "State",
        render: (row) => <StateBadge state={row.active} />,
    },
    { field: "load", label: "Load" },
    { field: "sub", label: "Sub" },
    { field: "description", label: "Description", truncate: true },
];

const ACTIONS: ReadonlyArray<RowAction<Unit>> = [
    {
        id: "restart",
        label: "Restart",
        method: "unit_action",
        args: (u) => ({ name: u.name, action: "restart" }),
        confirm: (u) => `Restart ${u.name}? In-flight requests on this unit will be interrupted.`,
    },
    {
        id: "start",
        label: "Start",
        method: "unit_action",
        args: (u) => ({ name: u.name, action: "start" }),
    },
    {
        id: "stop",
        label: "Stop",
        method: "unit_action",
        args: (u) => ({ name: u.name, action: "stop" }),
        confirm: (u) => `Stop ${u.name}?`,
        danger: true,
    },
    {
        id: "enable",
        label: "Enable on boot",
        method: "unit_action",
        args: (u) => ({ name: u.name, action: "enable" }),
    },
    {
        id: "disable",
        label: "Disable on boot",
        method: "unit_action",
        args: (u) => ({ name: u.name, action: "disable" }),
        danger: true,
    },
];

export function SystemdServices({ projectID, agentID, active }: PluginUIProps) {
    return (
        <RPCTable<ListUnitsResponse, Unit>
            projectID={projectID}
            agentID={agentID}
            pluginID={PLUGIN_ID}
            method="list_units"
            active={active}
            requestForm={[
                {
                    field: "state",
                    kind: "select",
                    label: "State",
                    options: [
                        { value: "", label: "All" },
                        { value: "active", label: "Active" },
                        { value: "inactive", label: "Inactive" },
                        { value: "failed", label: "Failed" },
                    ],
                    default: "",
                },
                {
                    field: "pattern",
                    kind: "search",
                    label: "Search",
                    default: "",
                    debounceMs: 200,
                    placeholder: "ssh.service",
                },
            ]}
            buildRequest={(form) => {
                const req: Record<string, unknown> = { unit_type: "service" };
                if (form.state) req.state = form.state;
                if (form.pattern) req.pattern = form.pattern;
                return req;
            }}
            rowsFrom={(r) => r.units ?? []}
            rowKey={(u) => u.name}
            columns={COLUMNS}
            actions={ACTIONS}
            refreshMs={30000}
            emptyText="No services match the current filters."
            pagination={{ kind: "offset" }}
        />
    );
}
