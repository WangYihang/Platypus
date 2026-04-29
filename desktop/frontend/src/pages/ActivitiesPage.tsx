import { useCallback, useEffect, useMemo, useState } from "react";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Download, Loader2, Search } from "lucide-react";
import { toast } from "sonner";

import EmptyState from "../components/EmptyState";
import FacetSidebar from "../components/FacetSidebar";
import RefreshButton from "../components/RefreshButton";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { getSessionUser } from "../lib/auth";
import { humanizeError } from "../lib/humanizeError";
import {
    ActivityItem,
    ActivityOutcome,
    ActivitySource,
    exportProjectActivitiesBlob,
    listProjectActivities,
    ListActivitiesOpts,
} from "../lib/api";
import { qk } from "../lib/queryKeys";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import { QuickFilterPreset, applyQuickFilter } from "./activities/quickFilters";
import {
    CATEGORY_OPTIONS,
    TimeRange,
    rangeToFrom,
} from "./activities/format";
import ActivityDetailSheet from "./activities/ActivityDetailSheet";
import ActivityTable from "./activities/ActivityTable";
import MultiSelectPopover from "./activities/MultiSelectPopover";
import QuickFilterChip from "./activities/QuickFilterChip";

// /projects/:slug/activities — paginated, filterable, project-scoped view
// of the unified activity log. Clicking a row opens a right-side sheet
// with the full structured meta payload.
export default function ActivitiesPage() {
    const project = useCurrentProject();
    const queryClient = useQueryClient();

    const [range, setRange] = useState<TimeRange>("7d");
    const [categories, setCategories] = useState<string[]>([]);
    // sources stays an array on the wire so a future "human + system" combo
    // doesn't need a new endpoint shape.
    const [sources, setSources] = useState<ActivitySource[]>([]);
    const [actor, setActor] = useState("");
    const [outcome, setOutcome] = useState<ActivityOutcome | "">("");
    const [query, setQuery] = useState("");
    const [includeGlobal, setIncludeGlobal] = useState(false);
    const [selected, setSelected] = useState<ActivityItem | null>(null);

    const me = getSessionUser();

    const onQuickFilter = useCallback(
        (preset: QuickFilterPreset) => {
            const patch = applyQuickFilter(preset, {
                username: me?.username ?? "",
            });
            if (patch.actor !== undefined) setActor(patch.actor);
            if (patch.outcome !== undefined) setOutcome(patch.outcome);
            if (patch.query !== undefined) setQuery(patch.query);
            if (patch.range !== undefined) setRange(patch.range);
            if (patch.categories !== undefined) setCategories(patch.categories);
            if (patch.sources !== undefined) setSources(patch.sources);
        },
        [me?.username],
    );

    const fromDate = useMemo(() => rangeToFrom(range), [range]);

    const buildOpts = useCallback(
        (cursor?: string): ListActivitiesOpts => ({
            from: fromDate ?? undefined,
            category: categories.length ? categories : undefined,
            sources: sources.length ? sources : undefined,
            actor: actor.trim() || undefined,
            outcome: outcome || undefined,
            q: query.trim() || undefined,
            includeGlobal,
            limit: 50,
            cursor,
            includeTotal: !cursor, // only fetch total on page 1
        }),
        [fromDate, categories, sources, actor, outcome, query, includeGlobal],
    );

    const baseOpts = useMemo(() => buildOpts(), [buildOpts]);
    const activitiesQuery = useInfiniteQuery({
        queryKey: qk.activities(project.id, baseOpts),
        queryFn: ({ pageParam }) =>
            listProjectActivities(
                project.id,
                buildOpts((pageParam as string | null) ?? undefined),
            ),
        initialPageParam: null as string | null,
        getNextPageParam: (last) => last.next_cursor ?? null,
    });

    const items = useMemo<ActivityItem[] | null>(() => {
        if (!activitiesQuery.data) return null;
        return activitiesQuery.data.pages.flatMap((p) => p.items);
    }, [activitiesQuery.data]);
    const total = activitiesQuery.data?.pages[0]?.total ?? null;
    const nextCursor: string | null =
        activitiesQuery.data?.pages.at(-1)?.next_cursor ?? null;
    const loading = activitiesQuery.isFetching && !activitiesQuery.isFetchingNextPage;
    const loadingMore = activitiesQuery.isFetchingNextPage;
    const error = activitiesQuery.error;

    const refresh = useCallback(() => {
        return queryClient.invalidateQueries({
            queryKey: qk.activities(project.id, baseOpts),
        });
    }, [queryClient, project.id, baseOpts]);

    const loadMore = useCallback(() => {
        if (!nextCursor) return;
        activitiesQuery.fetchNextPage().catch((e) => {
            toast.error(`load more: ${humanizeError(e)}`);
        });
    }, [activitiesQuery, nextCursor]);

    useEffect(() => {
        if (error) toast.error(`load activities: ${humanizeError(error)}`);
    }, [error]);

    // Facet sidebar — counts derived from currently-loaded items.
    const userFacetOptions = useMemo(() => {
        const counts = new Map<string, number>();
        for (const it of items ?? []) {
            const u = it.actor_user || "";
            if (!u) continue;
            counts.set(u, (counts.get(u) ?? 0) + 1);
        }
        return Array.from(counts.entries())
            .sort((a, b) => b[1] - a[1])
            .slice(0, 8)
            .map(([u, c]) => ({
                value: u,
                count: c,
                selected: actor.trim() === u,
            }));
    }, [items, actor]);
    const outcomeFacetOptions = useMemo(() => {
        const counts: Record<string, number> = {};
        for (const it of items ?? []) {
            counts[it.outcome] = (counts[it.outcome] ?? 0) + 1;
        }
        return (["success", "denied", "error"] as const)
            .filter((v) => (counts[v] ?? 0) > 0)
            .map((v) => ({
                value: v,
                count: counts[v] ?? 0,
                selected: outcome === v,
            }));
    }, [items, outcome]);
    const onFacetToggle = useCallback((key: string, value: string) => {
        if (key === "user") {
            setActor((prev) => (prev.trim() === value ? "" : value));
        } else if (key === "outcome") {
            setOutcome((prev) =>
                prev === value ? "" : (value as ActivityOutcome),
            );
        }
    }, []);

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
                toast.success(`Exported activities as ${format.toUpperCase()}`);
            } catch (e) {
                toast.error(`export: ${humanizeError(e)}`);
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
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
            <FacetSidebar
                facets={[
                    { key: "user", title: "User", options: userFacetOptions },
                    { key: "outcome", title: "Outcome", options: outcomeFacetOptions },
                ]}
                onToggle={onFacetToggle}
            />
            <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
                <div
                    data-testid="activities-quick-filters"
                    style={{
                        display: "flex",
                        flexWrap: "wrap",
                        gap: space[2],
                        padding: `${space[2]}px ${space[4]}px 0`,
                    }}
                >
                    <QuickFilterChip
                        label="My actions"
                        title="Filter to your actions over the last 24h"
                        disabled={!me?.username}
                        onClick={() => onQuickFilter("my")}
                    />
                    <QuickFilterChip
                        label="Failures"
                        title="Show only events with outcome = error"
                        onClick={() => onQuickFilter("failures")}
                    />
                    <QuickFilterChip
                        label="Last 24h"
                        title="Narrow the time window to the last 24 hours"
                        onClick={() => onQuickFilter("24h")}
                    />
                    <QuickFilterChip
                        label="Clear"
                        title="Reset every filter"
                        variant="ghost"
                        onClick={() => onQuickFilter("clear")}
                    />
                </div>
                <Toolbar
                    left={
                        <>
                            <ToggleGroup
                                type="single"
                                value={sources[0] ?? "all"}
                                variant="outline"
                                size="sm"
                                onValueChange={(v) => {
                                    if (!v || v === "all") setSources([]);
                                    else setSources([v as ActivitySource]);
                                }}
                            >
                                <ToggleGroupItem value="all" title="All sources">
                                    All
                                </ToggleGroupItem>
                                <ToggleGroupItem value="human" title="Actions initiated by a user or API token">
                                    Users
                                </ToggleGroupItem>
                                <ToggleGroupItem value="agent" title="Agent link lifecycle events">
                                    Agents
                                </ToggleGroupItem>
                                <ToggleGroupItem value="system" title="Server-side background events">
                                    System
                                </ToggleGroupItem>
                            </ToggleGroup>

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
                                {subtitle}
                            </span>
                            <span style={{ color: palette.textSecondary, fontSize: 12, marginLeft: 8 }}>
                                Include global
                            </span>
                            <Switch checked={includeGlobal} onCheckedChange={setIncludeGlobal} />
                            <Button size="sm" variant="outline" onClick={() => handleExport("jsonl")}>
                                <Download className="size-3.5" />
                                JSONL
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => handleExport("csv")}>
                                <Download className="size-3.5" />
                                CSV
                            </Button>
                            <RefreshButton
                                loading={loading}
                                onClick={refresh}
                                iconOnly
                                aria-label="Refresh"
                            />
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
                            {humanizeError(error)}
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
                        <ActivityTable
                            items={items}
                            nextCursor={nextCursor}
                            loadingMore={loadingMore}
                            onLoadMore={loadMore}
                            onSelect={setSelected}
                        />
                    )}
                </div>

                <ActivityDetailSheet
                    item={selected}
                    onOpenChange={(open) => {
                        if (!open) setSelected(null);
                    }}
                />
            </div>
        </div>
    );
}
