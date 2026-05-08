// Logs tab — shared across sys-journald-linux / sys-log-darwin /
// sys-log-windows. All three plugins implement `query` with the
// same JournalQueryRequest / JournalQueryResponse shape so one
// component covers the family. Per-OS variants only differ in
// the pluginID passed in by the registry.
//
// The form's "Unit" knob maps to the source filter (--unit on
// linux journalctl, process== on darwin `log show`, -ProviderName
// on windows Get-WinEvent). Priority is the journald 0..7 ladder
// (windows levels are pre-mapped by the plugin).

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Entry {
    timestampUs?: number;
    unit?: string;
    priority?: number;
    message?: string;
    hostname?: string;
    pid?: number;
    uid?: number;
    identifier?: string;
    comm?: string;
}

interface QueryResponse {
    entries?: Entry[];
    truncated?: boolean;
    error?: string;
}

const PRIORITY_COLOR: Record<number, string> = {
    0: palette.danger, // emerg
    1: palette.danger, // alert
    2: palette.danger, // crit
    3: palette.danger, // err
    4: palette.warning, // warning
    5: palette.warning, // notice
    6: palette.textMuted, // info
    7: palette.textMuted, // debug
};

const PRIORITY_LABEL: Record<number, string> = {
    0: "emerg",
    1: "alert",
    2: "crit",
    3: "err",
    4: "warn",
    5: "notice",
    6: "info",
    7: "debug",
};

function PriorityBadge({ p }: { p: number | undefined }) {
    if (p === undefined) return <>—</>;
    const color = PRIORITY_COLOR[p] ?? palette.textMuted;
    return (
        <span
            style={{
                display: "inline-block",
                padding: "1px 6px",
                borderRadius: 4,
                fontSize: 10,
                fontFamily: "monospace",
                fontWeight: 600,
                color: "#fff",
                background: color,
            }}
        >
            {PRIORITY_LABEL[p] ?? String(p)}
        </span>
    );
}

function Timestamp({ us }: { us: number | undefined }) {
    if (!us) return <>—</>;
    const d = new Date(us / 1000);
    return (
        <span
            style={{
                fontFamily: "monospace",
                fontSize: 11,
                color: palette.textMuted,
            }}
        >
            {d.toLocaleString()}
        </span>
    );
}

const COLUMNS: ReadonlyArray<Column<Entry>> = [
    {
        field: "timestampUs",
        label: "When",
        render: (row) => <Timestamp us={row.timestampUs} />,
    },
    {
        field: "priority",
        label: "Pri",
        render: (row) => <PriorityBadge p={row.priority} />,
    },
    {
        field: "unit",
        label: "Unit",
        render: (row) => row.unit ?? row.identifier ?? row.comm ?? "—",
    },
    {
        field: "message",
        label: "Message",
        truncate: true,
        primary: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {row.message ?? ""}
            </span>
        ),
    },
];

const PRIORITY_OPTIONS = [
    { value: "", label: "All" },
    { value: "0", label: "Emerg" },
    { value: "1", label: "Alert" },
    { value: "2", label: "Crit" },
    { value: "3", label: "Err" },
    { value: "4", label: "Warn" },
    { value: "5", label: "Notice" },
    { value: "6", label: "Info" },
    { value: "7", label: "Debug" },
];

export function SystemLogs({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    return (
        <RPCTable<QueryResponse, Entry>
            projectID={projectID}
            agentID={agentID}
            pluginID={pluginID}
            method="query"
            active={active}
            requestForm={[
                {
                    field: "priority",
                    kind: "select",
                    label: "Priority ≤",
                    options: PRIORITY_OPTIONS,
                    default: "",
                },
                {
                    field: "unit",
                    kind: "search",
                    label: "Unit",
                    default: "",
                    debounceMs: 200,
                    placeholder: "ssh.service",
                },
                {
                    field: "grep",
                    kind: "search",
                    label: "Grep",
                    default: "",
                    debounceMs: 300,
                    placeholder: "regex",
                },
                {
                    field: "since",
                    kind: "text",
                    label: "Since",
                    default: "1h ago",
                },
                {
                    field: "lines",
                    kind: "number",
                    label: "Lines",
                    default: 200,
                    min: 10,
                    max: 5000,
                    step: 50,
                },
            ]}
            buildRequest={(form) => ({
                ...(form.priority ? { priority: form.priority } : {}),
                ...(form.unit ? { unit: form.unit } : {}),
                ...(form.grep ? { grep: form.grep } : {}),
                ...(form.since ? { since: form.since } : {}),
                lines: Number(form.lines) || 200,
            })}
            rowsFrom={(r) => r.entries ?? []}
            rowKey={(e, idx) => `${e.timestampUs ?? idx}-${idx}`}
            columns={COLUMNS}
            refreshMs={0}
            emptyText="No matching journal entries."
        />
    );
}
