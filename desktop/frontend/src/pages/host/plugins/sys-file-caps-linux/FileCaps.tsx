// File capabilities tab — sys-file-caps-linux. Companion to the
// SUID outliers surfaced by sys-security: SUID is the classic
// privileged-bit, file capabilities are its modern replacement
// (cap_net_raw on ping, cap_dac_read_search on debug helpers).
//
// `list_file_caps` shells `getcap -r <root>` for the standard
// binary directories and pre-classifies risk. The default view
// hides allowlisted entries (ping, mtr, …) so operators see
// outliers up front; toggle to surface every cap'd file.

import RPCTable, { type Column } from "../shared/RPCTable";
import { Badge, type BadgeTone } from "../shared/Badge";
import { palette } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface FileCap {
    path?: string;
    caps?: string;
    allowlisted?: boolean;
    risk?: string;
}

interface ListFileCapsResponse {
    entries?: FileCap[];
    backend?: string;
    error?: string;
}

const RISK_TONE: Record<string, BadgeTone> = {
    low: "muted",
    medium: "warning",
    high: "danger",
};

function RiskBadge({ risk }: { risk: string }) {
    return (
        <Badge tone={RISK_TONE[risk.toLowerCase()] ?? "muted"} shape="tag">
            {risk || "—"}
        </Badge>
    );
}

const COLUMNS: ReadonlyArray<Column<FileCap>> = [
    {
        field: "path",
        label: "Binary",
        primary: true,
        truncate: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {row.path || "—"}
            </span>
        ),
    },
    {
        field: "caps",
        label: "Capabilities",
        truncate: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace", fontSize: 12 }}>
                {row.caps || "—"}
            </span>
        ),
    },
    {
        field: "risk",
        label: "Risk",
        render: (row) => <RiskBadge risk={row.risk ?? "low"} />,
    },
    {
        field: "allowlisted",
        label: "Known",
        render: (row) =>
            row.allowlisted ? (
                <span style={{ color: palette.textMuted }}>allowlisted</span>
            ) : (
                <span style={{ color: palette.warning }}>outlier</span>
            ),
    },
];

export function FileCaps({ projectID, agentID, active }: PluginUIProps) {
    return (
        <RPCTable<ListFileCapsResponse, FileCap>
            projectID={projectID}
            agentID={agentID}
            pluginID="com.platypus.sys-file-caps-linux"
            method="list_file_caps"
            active={active}
            requestForm={[
                {
                    field: "include_allowlisted",
                    kind: "toggle",
                    label: "Include allowlisted (ping, mtr, …)",
                    default: false,
                },
                {
                    field: "max_results",
                    kind: "number",
                    label: "Max",
                    default: 500,
                    min: 50,
                    max: 5000,
                    step: 50,
                },
            ]}
            buildRequest={(form) => ({
                include_allowlisted: Boolean(form.include_allowlisted),
                max_results: Number(form.max_results) || 500,
            })}
            rowsFrom={(r) => r.entries ?? []}
            rowKey={(c, idx) => c.path || `cap-${idx}`}
            columns={COLUMNS}
            refreshMs={0}
            emptyText="No cap'd binaries found (or getcap unavailable)."
        />
    );
}
