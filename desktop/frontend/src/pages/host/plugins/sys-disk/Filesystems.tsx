// Filesystems tab — shared across sys-disk-linux / -darwin / -windows.
// All three plugins emit the same protojson wire shape (same field
// names, same byte units), so one component covers the family.

import RPCTable, { type Column } from "../shared/RPCTable";
import type { PluginUIProps } from "../registry";

interface Filesystem {
    source: string;
    fstype: string;
    mountpoint: string;
    sizeBytes: number;
    usedBytes: number;
    availableBytes: number;
    percentUsed: number;
}

interface ListFilesystemsResponse {
    filesystems?: Filesystem[];
    error?: string;
}

const COLUMNS: ReadonlyArray<Column<Filesystem>> = [
    { field: "mountpoint", label: "Mounted on", primary: true },
    { field: "source", label: "Filesystem" },
    { field: "fstype", label: "Type" },
    {
        field: "sizeBytes",
        label: "Size",
        render: (row) => formatBytes(row.sizeBytes),
    },
    {
        field: "usedBytes",
        label: "Used",
        render: (row) => formatBytes(row.usedBytes),
    },
    {
        field: "availableBytes",
        label: "Avail",
        render: (row) => formatBytes(row.availableBytes),
    },
    {
        field: "percentUsed",
        label: "Use%",
        render: (row) => <span>{row.percentUsed}%</span>,
    },
];

function formatBytes(n: number): string {
    if (!n || n < 0) return "—";
    const units = ["B", "KB", "MB", "GB", "TB", "PB"];
    let v = n;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i += 1;
    }
    return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

export function Filesystems({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    return (
        <RPCTable<ListFilesystemsResponse, Filesystem>
            projectID={projectID}
            agentID={agentID}
            pluginID={pluginID}
            method="list_filesystems"
            active={active}
            requestForm={[
                {
                    field: "skip_pseudo",
                    kind: "toggle",
                    label: "Skip pseudo-FS",
                    default: true,
                },
            ]}
            buildRequest={(form) => ({
                skip_pseudo: Boolean(form.skip_pseudo),
            })}
            rowsFrom={(r) => r.filesystems ?? []}
            rowKey={(fs) => fs.mountpoint}
            columns={COLUMNS}
            refreshMs={60000}
            emptyText="No filesystems."
        />
    );
}
