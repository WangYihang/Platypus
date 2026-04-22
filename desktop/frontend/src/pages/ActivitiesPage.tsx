import * as React from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
    Clock,
    Download,
    Loader2,
    RotateCw,
    Search,
} from "lucide-react";
import { toast } from "sonner";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    ActivityItem,
    ActivityOutcome,
    exportProjectActivitiesBlob,
    listProjectActivities,
    ListActivitiesOpts,
} from "../lib/api";
import { fromNow } from "../lib/time";
import { cn } from "@/lib/cn";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "@/components/ui/tooltip";

type TimeRange = "24h" | "7d" | "30d" | "all";

// Curated superset of categories the backend writes. Kept static so the
// filter dropdown renders before the first fetch; the underlying `q`
// text search is the escape hatch for anything not in this list.
const CATEGORY_OPTIONS = [
    "auth",
    "session",
    "command",
    "file",
    "tunnel",
    "listener",
    "agent",
    "admin",
    "project",
    "server",
    "user",
];

const OUTCOME_TONE: Record<ActivityOutcome, "success" | "warning" | "danger"> = {
    success: "success",
    denied: "warning",
    error: "danger",
};

// ActivitiesPage renders /projects/:slug/activities: a paginated,
// filterable, project-scoped view of the unified activity log. Clicking
// a row opens a right-side sheet with the full structured meta payload
// so the table itself can stay compact.
export default function ActivitiesPage() {
    const project = useCurrentProject();
    const [items, setItems] = useState<ActivityItem[] | null>(null);
    const [nextCursor, setNextCursor] = useState<string | null>(null);
    const [total, setTotal] = useState<number | null>(null);
    const [loading, setLoading] = useState(false);
    const [loadingMore, setLoadingMore] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const [range, setRange] = useState<TimeRange>("7d");
    const [categories, setCategories] = useState<string[]>([]);
    const [actor, setActor] = useState("");
    const [outcome, setOutcome] = useState<ActivityOutcome | "">("");
    const [query, setQuery] = useState("");
    const [includeGlobal, setIncludeGlobal] = useState(false);
    const [selected, setSelected] = useState<ActivityItem | null>(null);

    const fromDate = useMemo(() => rangeToFrom(range), [range]);

    const buildOpts = useCallback(
        (cursor?: string): ListActivitiesOpts => ({
            from: fromDate ?? undefined,
            category: categories.length ? categories : undefined,
            actor: actor.trim() || undefined,
            outcome: outcome || undefined,
            q: query.trim() || undefined,
            includeGlobal,
            limit: 50,
            cursor,
            includeTotal: !cursor, // only fetch total on page 1
        }),
        [fromDate, categories, actor, outcome, query, includeGlobal],
    );

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const resp = await listProjectActivities(project.id, buildOpts());
            setItems(resp.items);
            setNextCursor(resp.next_cursor ?? null);
            setTotal(resp.total ?? null);
            setError(null);
        } catch (e) {
            setError(String(e));
            toast.error(`load activities: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id, buildOpts]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const loadMore = useCallback(async () => {
        if (!nextCursor) return;
        setLoadingMore(true);
        try {
            const resp = await listProjectActivities(project.id, buildOpts(nextCursor));
            setItems((prev) => [...(prev ?? []), ...resp.items]);
            setNextCursor(resp.next_cursor ?? null);
        } catch (e) {
            toast.error(`load more: ${String(e)}`);
        } finally {
            setLoadingMore(false);
        }
    }, [project.id, nextCursor, buildOpts]);

    const handleExport = useCallback(
        async (format: "jsonl" | "csv") => {
            try {
                const blob = await exportProjectActivitiesBlob(project.id, {
                    ...buildOpts(),
                    format,
                });
                const url = URL.createObjectURL(blob);
                const a = document.createElement("a");
                a.href = url;
                a.download = `activities-${project.slug}.${format}`;
                a.click();
                URL.revokeObjectURL(url);
            } catch (e) {
                toast.error(`export: ${String(e)}`);
            }
        },
        [project.id, project.slug, buildOpts],
    );

    const subtitle = useMemo(() => {
        if (items === null) return "Loading…";
        if (total !== null) return `${total.toLocaleString()} events`;
        return `${items.length.toLocaleString()} loaded`;
    }, [items, total]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Activities"
                subtitle={subtitle}
                actions={
                    <>
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() => handleExport("jsonl")}
                        >
                            <Download className="size-3.5" />
                            Export JSONL
                        </Button>
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() => handleExport("csv")}
                        >
                            <Download className="size-3.5" />
                            Export CSV
                        </Button>
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={loading}
                            onClick={refresh}
                        >
                            {loading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                    </>
                }
            />
            <Toolbar
                left={
                    <>
                        <ToggleGroup
                            type="single"
                            value={range}
                            variant="outline"
                            size="sm"
                            onValueChange={(v) => {
                                if (v) setRange(v as TimeRange);
                            }}
                        >
                            <ToggleGroupItem value="24h">24h</ToggleGroupItem>
                            <ToggleGroupItem value="7d">7d</ToggleGroupItem>
                            <ToggleGroupItem value="30d">30d</ToggleGroupItem>
                            <ToggleGroupItem value="all">All</ToggleGroupItem>
                        </ToggleGroup>

                        <MultiSelectPopover
                            label="Category"
                            options={CATEGORY_OPTIONS}
                            selected={categories}
                            onChange={setCategories}
                        />

                        <Select
                            value={outcome || "__all__"}
                            onValueChange={(v) =>
                                setOutcome(v === "__all__" ? "" : (v as ActivityOutcome))
                            }
                        >
                            <SelectTrigger size="sm" className="min-w-[140px]">
                                <SelectValue placeholder="Outcome" />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="__all__">All outcomes</SelectItem>
                                <SelectItem value="success">success</SelectItem>
                                <SelectItem value="denied">denied</SelectItem>
                                <SelectItem value="error">error</SelectItem>
                            </SelectContent>
                        </Select>

                        <Input
                            placeholder="Actor (username)"
                            value={actor}
                            onChange={(e) => setActor(e.target.value)}
                            className="h-8 max-w-[200px]"
                        />

                        <div className="relative max-w-[280px] w-full">
                            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                            <Input
                                placeholder="Search action / target / meta"
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                className="h-8 pl-8"
                            />
                        </div>
                    </>
                }
                right={
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span style={{ color: palette.textSecondary, fontSize: 12 }}>
                            Include global
                        </span>
                        <Switch checked={includeGlobal} onCheckedChange={setIncludeGlobal} />
                    </div>
                }
            />

            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <div
                        style={{
                            marginBottom: space[4],
                            padding: `${space[3]}px ${space[4]}px`,
                            border: `1px solid ${palette.danger}`,
                            borderRadius: 6,
                            color: palette.danger,
                            fontSize: 13,
                        }}
                    >
                        {error}
                    </div>
                )}
                {!items && (
                    <div
                        style={{
                            display: "flex",
                            justifyContent: "center",
                            alignItems: "center",
                            padding: 80,
                        }}
                    >
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                )}
                {items && items.length === 0 && (
                    <EmptyState
                        icon={<Clock className="size-5" />}
                        title="No activities yet"
                        description="As users, agents, and the server do things, their actions appear here in real time."
                    />
                )}
                {items && items.length > 0 && (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead className="w-[140px]">When</TableHead>
                                    <TableHead className="w-[180px]">Actor</TableHead>
                                    <TableHead className="w-[220px]">Action</TableHead>
                                    <TableHead>Target</TableHead>
                                    <TableHead className="w-[110px]">Outcome</TableHead>
                                    <TableHead className="w-[100px]">Duration</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {items.map((it) => (
                                    <TableRow
                                        key={it.id}
                                        onClick={() => setSelected(it)}
                                        className="cursor-pointer"
                                    >
                                        <TableCell>
                                            <Tooltip>
                                                <TooltipTrigger asChild>
                                                    <span className="text-text-secondary">
                                                        {fromNow(it.at)}
                                                    </span>
                                                </TooltipTrigger>
                                                <TooltipContent>
                                                    {new Date(it.at).toLocaleString()}
                                                </TooltipContent>
                                            </Tooltip>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex flex-col">
                                                <span className="text-text-primary">
                                                    {it.actor_user || (
                                                        <span className="text-text-muted">
                                                            system
                                                        </span>
                                                    )}
                                                </span>
                                                {it.actor_ip && (
                                                    <Mono style={{ fontSize: 11, color: palette.textMuted }}>
                                                        {it.actor_ip}
                                                    </Mono>
                                                )}
                                            </div>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex flex-col">
                                                <Mono>{it.action}</Mono>
                                                <span className="text-[11px] text-text-muted">
                                                    {it.category}
                                                </span>
                                            </div>
                                        </TableCell>
                                        <TableCell>
                                            <TargetCell item={it} />
                                        </TableCell>
                                        <TableCell>
                                            <StatusPill
                                                tone={OUTCOME_TONE[it.outcome] ?? "neutral"}
                                            >
                                                {it.outcome}
                                            </StatusPill>
                                        </TableCell>
                                        <TableCell>
                                            {typeof it.duration_ms === "number" ? (
                                                <span className="text-text-secondary">
                                                    {formatDuration(it.duration_ms)}
                                                </span>
                                            ) : (
                                                <span className="text-text-muted">—</span>
                                            )}
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                        {nextCursor && (
                            <div
                                style={{
                                    padding: space[4],
                                    display: "flex",
                                    justifyContent: "center",
                                }}
                            >
                                <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={loadingMore}
                                    onClick={loadMore}
                                >
                                    {loadingMore && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Load more
                                </Button>
                            </div>
                        )}
                    </Card>
                )}
            </div>

            <ActivityDetailSheet
                item={selected}
                onOpenChange={(open) => {
                    if (!open) setSelected(null);
                }}
            />
        </div>
    );
}

// TargetCell renders a truncated, tooltip-backed target label so long
// paths / URLs don't blow up the row. Split out from the main table
// body so it's obvious where the truncation rule lives.
function TargetCell({ item }: { item: ActivityItem }) {
    const label = item.target_label || item.target_id || "—";
    const short = label.length > 40 ? `${label.slice(0, 40)}…` : label;
    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <span>
                    <Mono>{short}</Mono>
                </span>
            </TooltipTrigger>
            <TooltipContent className="max-w-[420px] break-all">{label}</TooltipContent>
        </Tooltip>
    );
}

// MultiSelectPopover is a minimal multi-select built from shadcn
// Popover + Checkbox. shadcn's core Select only handles single values;
// rather than pulling in Command + cmdk just for checkboxes, a tiny
// popover list is enough for the ~10-option category filter.
function MultiSelectPopover({
    label,
    options,
    selected,
    onChange,
}: {
    label: string;
    options: string[];
    selected: string[];
    onChange: (next: string[]) => void;
}) {
    const [open, setOpen] = useState(false);
    const summary =
        selected.length === 0
            ? label
            : selected.length === 1
              ? selected[0]
              : `${label} · ${selected.length}`;

    function toggle(opt: string, next: boolean) {
        if (next) onChange([...selected, opt]);
        else onChange(selected.filter((x) => x !== opt));
    }

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button
                    variant="outline"
                    size="sm"
                    className={cn("min-w-[160px] justify-between", {
                        "text-text-muted": selected.length === 0,
                    })}
                >
                    {summary}
                </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-[220px] p-1">
                {options.map((opt) => {
                    const checked = selected.includes(opt);
                    return (
                        <label
                            key={opt}
                            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent"
                        >
                            <Checkbox
                                checked={checked}
                                onCheckedChange={(v) => toggle(opt, v === true)}
                            />
                            <span>{opt}</span>
                        </label>
                    );
                })}
                {selected.length > 0 && (
                    <>
                        <div className="my-1 h-px bg-border" />
                        <button
                            type="button"
                            onClick={() => onChange([])}
                            className="flex w-full items-center justify-center rounded-sm px-2 py-1.5 text-xs text-text-muted hover:bg-accent hover:text-text-primary"
                        >
                            Clear
                        </button>
                    </>
                )}
            </PopoverContent>
        </Popover>
    );
}

// ActivityDetailSheet slides a right-side panel in for one row. Kept
// separate so the table stays compact and the sheet can grow richer
// over time (links to session, request_id cross-reference, etc.).
function ActivityDetailSheet({
    item,
    onOpenChange,
}: {
    item: ActivityItem | null;
    onOpenChange: (open: boolean) => void;
}) {
    const rows: Array<{ label: string; value: React.ReactNode }> = [];
    if (item) {
        rows.push({ label: "When", value: new Date(item.at).toLocaleString() });
        rows.push({ label: "Action", value: <Mono>{item.action}</Mono> });
        rows.push({ label: "Category", value: <Mono>{item.category}</Mono> });
        rows.push({
            label: "Actor",
            value: (
                <div>
                    <div>{item.actor_user || "(system)"}</div>
                    {item.actor_ip && (
                        <Mono style={{ fontSize: 11, color: palette.textMuted }}>
                            {item.actor_ip}
                        </Mono>
                    )}
                    {item.actor_ua && (
                        <div className="text-[11px] text-text-muted">{item.actor_ua}</div>
                    )}
                </div>
            ),
        });
        rows.push({
            label: "Target",
            value: (
                <div>
                    {item.target_type && (
                        <span className="text-text-muted">{item.target_type} · </span>
                    )}
                    <Mono>{item.target_label || item.target_id || "—"}</Mono>
                </div>
            ),
        });
        rows.push({
            label: "Outcome",
            value: (
                <StatusPill tone={OUTCOME_TONE[item.outcome] ?? "neutral"}>
                    {item.outcome}
                </StatusPill>
            ),
        });
        if (item.error) rows.push({ label: "Error", value: <Mono>{item.error}</Mono> });
        if (typeof item.duration_ms === "number") {
            rows.push({ label: "Duration", value: formatDuration(item.duration_ms) });
        }
        if (item.session_id) rows.push({ label: "Session", value: <Mono>{item.session_id}</Mono> });
        if (item.request_id)
            rows.push({ label: "Request ID", value: <Mono>{item.request_id}</Mono> });
        if (item.project_id)
            rows.push({ label: "Project", value: <Mono>{item.project_id}</Mono> });
    }

    return (
        <Sheet open={item !== null} onOpenChange={onOpenChange}>
            <SheetContent className="w-[520px] sm:max-w-[520px] overflow-y-auto">
                <SheetHeader>
                    <SheetTitle>{item ? <Mono>{item.action}</Mono> : "Activity"}</SheetTitle>
                    <SheetDescription>Full structured record for this event.</SheetDescription>
                </SheetHeader>
                <div className="px-4 pb-6 space-y-4">
                    <div
                        className="grid gap-x-5"
                        style={{
                            gridTemplateColumns: "110px 1fr",
                            rowGap: 12,
                        }}
                    >
                        {rows.map((r) => (
                            <Fragment key={r.label} label={r.label} value={r.value} />
                        ))}
                    </div>
                    {item?.meta && (
                        <div>
                            <div className="mb-2 text-[11px] uppercase text-text-muted">
                                Meta
                            </div>
                            <pre className="max-h-[360px] overflow-auto rounded border border-border bg-surface p-4 text-xs text-text-primary">
                                {typeof item.meta === "string"
                                    ? item.meta
                                    : JSON.stringify(item.meta, null, 2)}
                            </pre>
                        </div>
                    )}
                </div>
            </SheetContent>
        </Sheet>
    );
}

function Fragment({ label, value }: { label: string; value: React.ReactNode }) {
    return (
        <>
            <div className="pt-0.5 text-[11px] uppercase text-text-muted">{label}</div>
            <div className="text-text-primary break-words">{value}</div>
        </>
    );
}

// rangeToFrom maps the toggle-group value to a lower-bound Date. "all"
// returns null so the server's "no from filter" branch kicks in.
function rangeToFrom(range: TimeRange): Date | null {
    const now = Date.now();
    switch (range) {
        case "24h":
            return new Date(now - 24 * 60 * 60 * 1000);
        case "7d":
            return new Date(now - 7 * 24 * 60 * 60 * 1000);
        case "30d":
            return new Date(now - 30 * 24 * 60 * 60 * 1000);
        case "all":
            return null;
    }
}

// formatDuration renders a millisecond count in the most compact form a
// human can eyeball: <1s stays as ms, 1s-60s as seconds with one
// decimal, above that as minutes+seconds.
function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms} ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
    const mins = Math.floor(ms / 60_000);
    const secs = Math.floor((ms % 60_000) / 1000);
    return `${mins}m ${secs}s`;
}
