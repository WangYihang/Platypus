// Packages tab — sys-pkg-{linux,darwin,windows}. Two RPCs:
//   list_installed → table with search + max_results form
//   list_upgradable → table of pending updates
//
// The component renders a top-level toggle to swap between them.
// Per-OS variants share schema; the Linux backend auto-detects
// apt/dnf/yum/zypper/pacman and reports its choice in the
// `backend` field, which we surface in the empty-state text.

import { useState } from "react";

import RPCTable, { type Column } from "../shared/RPCTable";
import { palette, space } from "../../../../layout/theme";
import type { PluginUIProps } from "../registry";

interface Package {
    name: string;
    version: string;
    arch?: string;
}

interface ListInstalledResponse {
    packages?: Package[];
    backend?: string;
    truncatedAt?: number;
    error?: string;
}

interface Update {
    name: string;
    currentVersion?: string;
    availableVersion: string;
}

interface ListUpgradableResponse {
    updates?: Update[];
    backend?: string;
    error?: string;
}

const INSTALLED_COLUMNS: ReadonlyArray<Column<Package>> = [
    { field: "name", label: "Package", primary: true },
    { field: "version", label: "Version" },
    {
        field: "arch",
        label: "Arch",
        render: (row) => row.arch ?? "—",
    },
];

const UPDATE_COLUMNS: ReadonlyArray<Column<Update>> = [
    { field: "name", label: "Package", primary: true },
    {
        field: "currentVersion",
        label: "Installed",
        render: (row) => row.currentVersion ?? "—",
    },
    { field: "availableVersion", label: "Available" },
];

type View = "installed" | "upgradable";

export function Packages({
    pluginID,
    projectID,
    agentID,
    active,
}: PluginUIProps & { pluginID: string }) {
    const [view, setView] = useState<View>("installed");

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <ViewToggle value={view} onChange={setView} />

            {view === "installed" ? (
                <RPCTable<ListInstalledResponse, Package>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_installed"
                    active={active}
                    requestForm={[
                        {
                            field: "query",
                            kind: "search",
                            label: "Search",
                            default: "",
                            debounceMs: 300,
                            placeholder: "openssl",
                        },
                        {
                            field: "max_results",
                            kind: "number",
                            label: "Max",
                            default: 500,
                            min: 10,
                            max: 5000,
                            step: 100,
                        },
                    ]}
                    buildRequest={(form) => ({
                        ...(form.query ? { query: form.query } : {}),
                        max_results: Number(form.max_results) || 500,
                    })}
                    rowsFrom={(r) => r.packages ?? []}
                    rowKey={(p) => p.name}
                    columns={INSTALLED_COLUMNS}
                    refreshMs={0}
                    emptyText="No matching packages."
                />
            ) : (
                <RPCTable<ListUpgradableResponse, Update>
                    projectID={projectID}
                    agentID={agentID}
                    pluginID={pluginID}
                    method="list_upgradable"
                    active={active}
                    rowsFrom={(r) => r.updates ?? []}
                    rowKey={(u) => u.name}
                    columns={UPDATE_COLUMNS}
                    refreshMs={0}
                    emptyText="Nothing to upgrade."
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
            {(["installed", "upgradable"] as const).map((v) => (
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
                    {v === "installed" ? "Installed" : "Upgradable"}
                </button>
            ))}
        </div>
    );
}
