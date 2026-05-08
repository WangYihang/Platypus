// Users tab — shared across sys-users-{linux,darwin,windows}.
// Single RPC `list_users` returns three arrays in one response:
//   users[]    — local accounts
//   groups[]   — local groups
//   sudoers[]  — sudo / Administrators escalation rows
//
// We surface them as three sub-views (segmented toggle), each
// rendered as its own RPCTable. The same RPC fires per view; the
// payload is small so the duplicate fetch is acceptable in v1
// (RPCTable's react-query cache also dedupes identical requests).

import { useState } from "react";

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette, space } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface User {
    username?: string;
    uid?: number;
    gid?: number;
    fullName?: string;
    home?: string;
    shell?: string;
    isSystem?: boolean;
    isLocked?: boolean;
    groups?: string[];
}

interface Group {
    name?: string;
    gid?: number;
    members?: string[];
}

interface SudoEntry {
    source?: string;
    who?: string;
    how?: string;
}

interface ListUsersResponse {
    users?: User[];
    groups?: Group[];
    sudoers?: SudoEntry[];
    error?: string;
}

const USER_COLUMNS: ReadonlyArray<Column<User>> = [
    { field: "username", label: "User", primary: true },
    {
        field: "uid",
        label: "UID",
        render: (row) => (row.uid !== undefined ? String(row.uid) : "—"),
    },
    {
        field: "fullName",
        label: "Full name",
        truncate: true,
        render: (row) => row.fullName || "—",
    },
    {
        field: "shell",
        label: "Shell",
        truncate: true,
        render: (row) => row.shell || "—",
    },
    {
        field: "home",
        label: "Home",
        truncate: true,
        render: (row) => row.home || "—",
    },
    {
        field: "isLocked",
        label: "Status",
        render: (row) =>
            row.isLocked ? (
                <span style={{ color: palette.warning }}>locked</span>
            ) : (
                <span style={{ color: palette.success }}>active</span>
            ),
    },
    {
        field: "groups",
        label: "Groups",
        truncate: true,
        render: (row) =>
            row.groups && row.groups.length > 0
                ? row.groups.join(", ")
                : "—",
    },
];

const GROUP_COLUMNS: ReadonlyArray<Column<Group>> = [
    { field: "name", label: "Group", primary: true },
    {
        field: "gid",
        label: "GID",
        render: (row) => (row.gid !== undefined ? String(row.gid) : "—"),
    },
    {
        field: "members",
        label: "Members",
        truncate: true,
        render: (row) =>
            row.members && row.members.length > 0
                ? row.members.join(", ")
                : "—",
    },
];

const SUDO_COLUMNS: ReadonlyArray<Column<SudoEntry>> = [
    { field: "who", label: "Who", primary: true },
    {
        field: "how",
        label: "How",
        truncate: true,
        render: (row) => row.how || "—",
    },
    {
        field: "source",
        label: "Source",
        truncate: true,
        render: (row) => row.source || "—",
    },
];

type View = "users" | "groups" | "sudoers";

export function Users({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    const [view, setView] = useState<View>("users");

    const requestForm = [
        {
            field: "include_system",
            kind: "toggle" as const,
            label: "Include system accounts",
            default: false,
        },
    ];
    const buildRequest = (form: Record<string, unknown>) => ({
        include_system: Boolean(form.include_system),
    });

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <ViewToggle value={view} onChange={setView} />

            {view === "users" ? (
                <RPCTable<ListUsersResponse, User>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_users"
                    active={active}
                    requestForm={requestForm}
                    buildRequest={buildRequest}
                    rowsFrom={(r) => r.users ?? []}
                    rowKey={(u, idx) => u.username || `user-${idx}`}
                    columns={USER_COLUMNS}
                    refreshMs={0}
                    emptyText="No matching accounts. Toggle 'Include system' to see service accounts."
                />
            ) : view === "groups" ? (
                <RPCTable<ListUsersResponse, Group>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_users"
                    active={active}
                    requestForm={requestForm}
                    buildRequest={buildRequest}
                    rowsFrom={(r) => r.groups ?? []}
                    rowKey={(g, idx) => g.name || `group-${idx}`}
                    columns={GROUP_COLUMNS}
                    refreshMs={0}
                    emptyText="No groups."
                />
            ) : (
                <RPCTable<ListUsersResponse, SudoEntry>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_users"
                    active={active}
                    requestForm={requestForm}
                    buildRequest={buildRequest}
                    rowsFrom={(r) => r.sudoers ?? []}
                    rowKey={(s, idx) =>
                        `${s.source ?? ""}-${s.who ?? ""}-${s.how ?? ""}-${idx}`
                    }
                    columns={SUDO_COLUMNS}
                    refreshMs={0}
                    emptyText="No sudoers / Administrators entries."
                />
            )}
        </div>
    );
}

function ViewToggle({
    value,
    onChange,
}: {
    value: View;
    onChange: (v: View) => void;
}) {
    return (
        <div
            role="tablist"
            style={{
                display: "inline-flex",
                gap: 2,
                background: palette.surfaceHover,
                padding: 2,
                borderRadius: 6,
                width: "fit-content",
            }}
        >
            {(["users", "groups", "sudoers"] as const).map((v) => (
                <button
                    key={v}
                    role="tab"
                    aria-selected={value === v}
                    onClick={() => onChange(v)}
                    style={{
                        padding: "4px 12px",
                        fontSize: 12,
                        fontWeight: 500,
                        borderRadius: 4,
                        border: "none",
                        cursor: "pointer",
                        background:
                            value === v ? palette.surface : "transparent",
                        color:
                            value === v ? palette.textPrimary : palette.textMuted,
                    }}
                >
                    {v[0]!.toUpperCase() + v.slice(1)}
                </button>
            ))}
        </div>
    );
}
