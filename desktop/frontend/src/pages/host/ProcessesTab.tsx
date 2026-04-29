import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import Mono from "../../components/Mono";
import RefreshButton from "../../components/RefreshButton";
import { palette, space } from "../../layout/theme";
import {
    HostProcess,
    ListHostProcessesOpts,
    listHostProcesses,
} from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";
import { fromNow } from "../../lib/time";

import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

interface Props {
    projectID: string;
    hostID: string;
    // `active` is true while the Processes tab is the visible one —
    // gates the 5s polling timer so we don't keep an offscreen tab
    // hammering the agent.
    active: boolean;
}

type SortKey = NonNullable<ListHostProcessesOpts["sort"]>;

// ProcessesTab renders a live, sortable process list from the agent.
// The server proxies a ProcessList RPC each time we refresh; nothing
// is cached DB-side, so an offline agent surfaces as an inline error
// rather than a stale list. Auto-refresh runs every 5 seconds while
// the tab is active.
export default function ProcessesTab({ projectID, hostID, active }: Props) {
    const [sort, setSort] = useState<SortKey>("cpu");
    const [top, setTop] = useState<number>(100);
    const [search, setSearch] = useState("");

    // react-query handles the abort-on-unmount, dedup, and retry
    // wiring we used to spell out by hand. `enabled: active` pauses
    // the query while the Processes tab is dormant; `refetchInterval:
    // active ? 5000 : false` matches the previous setInterval gate.
    // When the tab comes back active, the query automatically
    // refetches and resumes polling — no explicit wake-up needed.
    const {
        data,
        isFetching: loading,
        error,
        refetch,
    } = useQuery({
        queryKey: [...qk.hostProcesses(projectID, hostID), { top, sort }] as const,
        queryFn: () => listHostProcesses(projectID, hostID, { top, sort }),
        enabled: active,
        refetchInterval: active ? 5000 : false,
        refetchIntervalInBackground: false,
    });

    const procs = data?.processes || [];
    const filtered = useMemo(() => {
        const needle = search.trim().toLowerCase();
        if (!needle) return procs;
        return procs.filter((p) => {
            return (
                p.name?.toLowerCase().includes(needle) ||
                p.user?.toLowerCase().includes(needle) ||
                p.cmdline?.toLowerCase().includes(needle) ||
                String(p.pid).includes(needle)
            );
        });
    }, [procs, search]);

    const headerLabel = (key: SortKey, label: string) => {
        const isActive = sort === key;
        return (
            <button
                onClick={() => setSort(key)}
                style={{
                    all: "unset",
                    cursor: "pointer",
                    color: isActive ? palette.textPrimary : palette.textSecondary,
                    fontWeight: isActive ? 600 : 500,
                }}
            >
                {label}
                {isActive ? " ↓" : ""}
            </button>
        );
    };

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <div
                style={{
                    display: "flex",
                    gap: space[3],
                    alignItems: "center",
                    flexWrap: "wrap",
                }}
            >
                <input
                    type="search"
                    placeholder="filter by pid / name / user / cmdline"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    style={{
                        flex: "1 1 260px",
                        minWidth: 240,
                        padding: `${space[2]}px ${space[3]}px`,
                        borderRadius: 6,
                        border: `1px solid ${palette.border}`,
                        background: palette.surface,
                        color: palette.textPrimary,
                        fontSize: 13,
                    }}
                />
                <label
                    style={{
                        display: "inline-flex",
                        gap: space[2],
                        alignItems: "center",
                        fontSize: 12,
                        color: palette.textSecondary,
                    }}
                >
                    top
                    <select
                        value={top}
                        onChange={(e) => setTop(Number(e.target.value))}
                        style={{
                            padding: `${space[1]}px ${space[2]}px`,
                            borderRadius: 4,
                            border: `1px solid ${palette.border}`,
                            background: palette.surface,
                            color: palette.textPrimary,
                            fontSize: 12,
                        }}
                    >
                        {[30, 100, 200, 500].map((n) => (
                            <option key={n} value={n}>
                                {n}
                            </option>
                        ))}
                    </select>
                </label>
                <RefreshButton loading={loading} onClick={() => void refetch()} />
                <span style={{ fontSize: 12, color: palette.textSecondary }}>
                    {data?.total_count !== undefined
                        ? `${filtered.length} of ${data.total_count}`
                        : loading
                        ? "loading…"
                        : "—"}
                </span>
            </div>

            {error && (
                <div
                    style={{
                        padding: `${space[3]}px ${space[4]}px`,
                        border: `1px solid ${palette.danger}`,
                        borderRadius: 6,
                        color: palette.danger,
                        fontSize: 12,
                    }}
                >
                    {humanizeError(error)}
                </div>
            )}

            <Table>
                <TableHeader>
                    <TableRow>
                        <TableHead className="w-[80px]">{headerLabel("pid", "pid")}</TableHead>
                        <TableHead className="w-[140px]">user</TableHead>
                        <TableHead>name</TableHead>
                        <TableHead className="w-[80px]">{headerLabel("cpu", "cpu %")}</TableHead>
                        <TableHead className="w-[80px]">{headerLabel("mem", "mem %")}</TableHead>
                        <TableHead className="w-[110px]">{headerLabel("rss", "rss")}</TableHead>
                        <TableHead className="w-[80px]">threads</TableHead>
                        <TableHead className="w-[140px]">started</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {filtered.map((p) => (
                        <ProcessRow key={p.pid} p={p} />
                    ))}
                </TableBody>
            </Table>
        </div>
    );
}

function ProcessRow({ p }: { p: HostProcess }) {
    const cmd = p.cmdline?.trim() || p.name || "";
    return (
        <TableRow title={cmd || undefined}>
            <TableCell>
                <Mono>{p.pid}</Mono>
            </TableCell>
            <TableCell>
                <Mono size={11}>{p.user || "—"}</Mono>
            </TableCell>
            <TableCell>
                <span style={{ display: "inline-flex", flexDirection: "column" }}>
                    <Mono>{p.name || "—"}</Mono>
                    {p.cmdline && p.cmdline !== p.name && (
                        <span
                            style={{
                                fontSize: 11,
                                color: palette.textSecondary,
                                maxWidth: 480,
                                whiteSpace: "nowrap",
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                            }}
                        >
                            {p.cmdline}
                        </span>
                    )}
                </span>
            </TableCell>
            <TableCell>{fmtPct(p.cpu_percent)}</TableCell>
            <TableCell>{fmtPct(p.mem_percent)}</TableCell>
            <TableCell>{fmtBytes(p.rss_bytes)}</TableCell>
            <TableCell>{p.num_threads ?? "—"}</TableCell>
            <TableCell
                style={{
                    color: palette.textSecondary,
                    fontSize: 12,
                }}
            >
                {p.created_at_unix
                    ? fromNow(new Date(p.created_at_unix * 1000).toISOString())
                    : "—"}
            </TableCell>
        </TableRow>
    );
}

function fmtPct(n?: number): string {
    if (n == null) return "—";
    return `${n.toFixed(1)}`;
}

function fmtBytes(n?: number): string {
    if (!n || n <= 0) return "—";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let v = n;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}
