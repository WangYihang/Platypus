// Firewall tab — shared across sys-firewall-{linux,darwin,windows}.
// Single RPC `list_firewall_rules` returns the unified FirewallRule
// shape regardless of backend (iptables / nftables / ufw / firewalld
// on linux, pf on darwin, Get-NetFirewallRule on windows). The
// detected backend is surfaced in the empty-state line.

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface FirewallRule {
    id?: string;
    name?: string;
    direction?: string;
    action?: string;
    protocol?: string;
    src?: string;
    dst?: string;
    srcPort?: string;
    dstPort?: string;
    enabled?: boolean;
    interface?: string;
    profile?: string;
    raw?: string;
    chain?: string;
}

interface ListFirewallRulesResponse {
    rules?: FirewallRule[];
    backend?: string;
    error?: string;
    totalCount?: number;
    hasMore?: boolean;
}

const ACTION_COLOR: Record<string, string> = {
    allow: palette.success,
    pass: palette.success,
    deny: palette.danger,
    drop: palette.danger,
    reject: palette.danger,
    block: palette.danger,
    log: palette.warning,
    masquerade: palette.warning,
};

function ActionBadge({ action }: { action: string }) {
    const color = ACTION_COLOR[action.toLowerCase()] ?? palette.textMuted;
    return (
        <span
            style={{
                display: "inline-block",
                padding: "1px 8px",
                borderRadius: 4,
                fontSize: 11,
                fontWeight: 600,
                color: "#fff",
                background: color,
            }}
        >
            {action || "—"}
        </span>
    );
}

const COLUMNS: ReadonlyArray<Column<FirewallRule>> = [
    {
        field: "name",
        label: "Rule",
        primary: true,
        render: (row) => row.name || row.chain || row.id || "—",
    },
    {
        field: "direction",
        label: "Dir",
        render: (row) => row.direction || "—",
    },
    {
        field: "action",
        label: "Action",
        render: (row) => <ActionBadge action={row.action ?? ""} />,
    },
    {
        field: "protocol",
        label: "Proto",
        render: (row) => row.protocol || "—",
    },
    {
        field: "src",
        label: "Source",
        truncate: true,
        render: (row) =>
            joinAddrPort(row.src, row.srcPort),
    },
    {
        field: "dst",
        label: "Destination",
        truncate: true,
        render: (row) =>
            joinAddrPort(row.dst, row.dstPort),
    },
    {
        field: "enabled",
        label: "On",
        render: (row) =>
            row.enabled === false ? (
                <span style={{ color: palette.textMuted }}>off</span>
            ) : (
                <span style={{ color: palette.success }}>on</span>
            ),
    },
    {
        field: "profile",
        label: "Profile",
        render: (row) => row.profile || "—",
    },
];

function joinAddrPort(addr: string | undefined, port: string | undefined) {
    const a = addr && addr !== "" ? addr : "any";
    if (!port || port === "" || port === "any") return a;
    return `${a}:${port}`;
}

export function Firewall({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    return (
        <RPCTable<ListFirewallRulesResponse, FirewallRule>
            projectID={projectID}
            agentID={agentID}
            pluginID={pluginID}
            method="list_firewall_rules"
            active={active}
            requestForm={[
                {
                    field: "filter",
                    kind: "search",
                    label: "Search",
                    default: "",
                    debounceMs: 250,
                    placeholder: "ssh / docker",
                },
                {
                    field: "include_disabled",
                    kind: "toggle",
                    label: "Include disabled",
                    default: false,
                },
                {
                    field: "limit",
                    kind: "number",
                    label: "Limit",
                    default: 200,
                    min: 50,
                    max: 5000,
                    step: 50,
                },
            ]}
            buildRequest={(form) => ({
                ...(form.filter ? { filter: form.filter } : {}),
                include_disabled: Boolean(form.include_disabled),
                limit: Number(form.limit) || 200,
            })}
            rowsFrom={(r) => r.rules ?? []}
            rowKey={(r, idx) =>
                r.id || `${r.chain ?? ""}-${r.name ?? ""}-${idx}`
            }
            columns={COLUMNS}
            refreshMs={0}
            emptyText="No firewall rules detected (or backend unavailable)."
        />
    );
}
