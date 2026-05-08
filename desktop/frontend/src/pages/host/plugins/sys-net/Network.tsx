// Network tab — sys-net-{linux,darwin,windows} share the same wire
// shape. The component renders a top-level toggle: Listeners |
// Connections, swapping the underlying RPC method.

import { useState } from "react";

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette, space } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Listener {
    proto: string;
    localAddr: string;
    localPort: number;
    pid?: number;
    comm?: string;
}

interface Connection {
    proto: string;
    localAddr: string;
    localPort: number;
    peerAddr?: string;
    peerPort?: number;
    state: string;
    pid?: number;
}

interface ListenersResponse {
    listeners?: Listener[];
    error?: string;
    totalCount?: number;
    hasMore?: boolean;
}

interface ConnectionsResponse {
    connections?: Connection[];
    error?: string;
    totalCount?: number;
    hasMore?: boolean;
}

const LISTENER_COLUMNS: ReadonlyArray<Column<Listener>> = [
    {
        field: "localPort",
        label: "Port",
        primary: true,
        render: (row) => <span style={{ fontFamily: "monospace" }}>{row.localPort}</span>,
    },
    { field: "localAddr", label: "Local Addr" },
    { field: "proto", label: "Proto" },
    {
        field: "comm",
        label: "Process",
        render: (row) => row.comm ?? "—",
    },
];

const CONNECTION_COLUMNS: ReadonlyArray<Column<Connection>> = [
    {
        field: "localPort",
        label: "Local",
        primary: true,
        render: (row) => (
            <span style={{ fontFamily: "monospace" }}>
                {row.localAddr}:{row.localPort}
            </span>
        ),
    },
    {
        field: "peerPort",
        label: "Peer",
        render: (row) =>
            row.peerAddr && row.peerPort
                ? `${row.peerAddr}:${row.peerPort}`
                : "—",
    },
    { field: "state", label: "State" },
    { field: "proto", label: "Proto" },
];

const STATE_OPTIONS = [
    { value: "", label: "Any" },
    { value: "ESTABLISHED", label: "Established" },
    { value: "TIME_WAIT", label: "Time-wait" },
    { value: "CLOSE_WAIT", label: "Close-wait" },
    { value: "LISTEN", label: "Listen" },
];

type View = "listeners" | "connections";

export function Network({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    const [view, setView] = useState<View>("listeners");

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <ViewToggle value={view} onChange={setView} />

            {view === "listeners" ? (
                <RPCTable<ListenersResponse, Listener>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_listeners"
                    active={active}
                    rowsFrom={(r) => r.listeners ?? []}
                    rowKey={(l) =>
                        `${l.proto}:${l.localAddr}:${l.localPort}`
                    }
                    columns={LISTENER_COLUMNS}
                    refreshMs={15000}
                    emptyText="No listening sockets."
                    pagination={{ kind: "offset" }}
                />
            ) : (
                <RPCTable<ConnectionsResponse, Connection>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_connections"
                    active={active}
                    requestForm={[
                        {
                            field: "state",
                            kind: "select",
                            label: "State",
                            options: STATE_OPTIONS,
                            default: "",
                        },
                    ]}
                    buildRequest={(form) =>
                        form.state ? { state: form.state } : {}
                    }
                    rowsFrom={(r) => r.connections ?? []}
                    rowKey={(c) =>
                        `${c.proto}:${c.localAddr}:${c.localPort}:${c.peerAddr ?? ""}:${c.peerPort ?? ""}`
                    }
                    columns={CONNECTION_COLUMNS}
                    refreshMs={15000}
                    emptyText="No connections match."
                    pagination={{ kind: "offset" }}
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
            {(["listeners", "connections"] as const).map((v) => (
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
                    {v === "listeners" ? "Listeners" : "Connections"}
                </button>
            ))}
        </div>
    );
}
