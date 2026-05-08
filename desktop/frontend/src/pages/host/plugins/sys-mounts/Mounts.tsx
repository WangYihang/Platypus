// Mounts tab — shared across sys-mounts-{linux,darwin,windows}.
// `list_mounts` returns two arrays in the same response:
//   mounts[]  — currently active kernel mounts
//   fstab[]   — /etc/fstab rows (linux only; empty on darwin/windows)
//
// Operators usually want one or the other; we surface a segmented
// toggle. The fstab tab is hidden when the response carries no
// fstab entries (i.e. on darwin / windows) — better than showing
// an "always empty" view.

import { useState } from "react";

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette, space } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Mount {
    source?: string;
    mountpoint?: string;
    fstype?: string;
    options?: string;
    readOnly?: boolean;
    nosuid?: boolean;
    nodev?: boolean;
    noexec?: boolean;
    fsId?: string;
    pseudo?: boolean;
}

interface FstabEntry {
    source?: string;
    mountpoint?: string;
    fstype?: string;
    options?: string;
    dump?: number;
    pass?: number;
    mounted?: boolean;
}

interface ListMountsResponse {
    mounts?: Mount[];
    fstab?: FstabEntry[];
    error?: string;
}

function FlagPill({ on, label }: { on: boolean | undefined; label: string }) {
    return (
        <span
            style={{
                display: "inline-block",
                padding: "1px 6px",
                borderRadius: 3,
                fontSize: 10,
                fontFamily: "monospace",
                fontWeight: 600,
                marginRight: 4,
                color: on ? "#fff" : palette.textMuted,
                background: on ? palette.warning : "transparent",
                border: on ? "none" : `1px solid ${palette.border}`,
            }}
        >
            {label}
        </span>
    );
}

const MOUNT_COLUMNS: ReadonlyArray<Column<Mount>> = [
    { field: "mountpoint", label: "Mounted on", primary: true },
    {
        field: "source",
        label: "Source",
        truncate: true,
        render: (row) => row.source || "—",
    },
    { field: "fstype", label: "Type" },
    {
        field: "readOnly",
        label: "Flags",
        render: (row) => (
            <span>
                {row.readOnly ? <FlagPill on label="ro" /> : null}
                {row.nosuid ? <FlagPill on label="nosuid" /> : null}
                {row.nodev ? <FlagPill on label="nodev" /> : null}
                {row.noexec ? <FlagPill on label="noexec" /> : null}
                {!row.readOnly && !row.nosuid && !row.nodev && !row.noexec
                    ? "—"
                    : null}
            </span>
        ),
    },
    {
        field: "options",
        label: "Options",
        truncate: true,
        render: (row) => row.options || "—",
    },
    {
        field: "pseudo",
        label: "Kind",
        render: (row) => (row.pseudo ? "pseudo" : "real"),
    },
];

const FSTAB_COLUMNS: ReadonlyArray<Column<FstabEntry>> = [
    { field: "mountpoint", label: "Mountpoint", primary: true },
    {
        field: "source",
        label: "Source",
        truncate: true,
        render: (row) => row.source || "—",
    },
    { field: "fstype", label: "Type" },
    {
        field: "options",
        label: "Options",
        truncate: true,
        render: (row) => row.options || "—",
    },
    {
        field: "mounted",
        label: "Mounted",
        render: (row) =>
            row.mounted ? (
                <span style={{ color: palette.success }}>yes</span>
            ) : (
                <span style={{ color: palette.warning }}>no</span>
            ),
    },
];

type View = "mounts" | "fstab";

export function Mounts({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    const [view, setView] = useState<View>("mounts");

    const requestForm = [
        {
            field: "include_pseudo",
            kind: "toggle" as const,
            label: "Include pseudo-FS",
            default: false,
        },
        {
            field: "include_active_fstab",
            kind: "toggle" as const,
            label: "Include mounted fstab rows",
            default: false,
        },
    ];
    const buildRequest = (form: Record<string, unknown>) => ({
        include_pseudo: Boolean(form.include_pseudo),
        include_active_fstab: Boolean(form.include_active_fstab),
    });

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <ViewToggle value={view} onChange={setView} />

            {view === "mounts" ? (
                <RPCTable<ListMountsResponse, Mount>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_mounts"
                    active={active}
                    requestForm={requestForm}
                    buildRequest={buildRequest}
                    rowsFrom={(r) => r.mounts ?? []}
                    rowKey={(m, idx) =>
                        `${m.mountpoint ?? ""}-${m.source ?? ""}-${idx}`
                    }
                    columns={MOUNT_COLUMNS}
                    refreshMs={0}
                    emptyText="No mounts (toggle 'Include pseudo-FS' to see virtual mounts)."
                />
            ) : (
                <RPCTable<ListMountsResponse, FstabEntry>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_mounts"
                    active={active}
                    requestForm={requestForm}
                    buildRequest={buildRequest}
                    rowsFrom={(r) => r.fstab ?? []}
                    rowKey={(f, idx) =>
                        `${f.mountpoint ?? ""}-${f.source ?? ""}-${idx}`
                    }
                    columns={FSTAB_COLUMNS}
                    refreshMs={0}
                    emptyText="No fstab entries (linux only; empty on darwin/windows)."
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
            {(["mounts", "fstab"] as const).map((v) => (
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
                    {v === "mounts" ? "Mounts" : "fstab"}
                </button>
            ))}
        </div>
    );
}
