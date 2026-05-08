// <RPCTable> — shared primitive for system-plugin "list things, take
// row-actions on a thing" tabs. One generic React component covers
// ~80% of the per-host plugin UI shape; each plugin's tab is a thin
// wrapper that passes column / action / request-form configuration
// as TypeScript values (no DSL, no JSONPath strings, no runtime
// schema validation).
//
// See PLAN /root/.claude/plans/abi-tdd-noble-firefly.md (Sprint 3 /
// N-phase) for the architectural rationale; the short version is:
// system plugins are written by us in the same monorepo, so adding
// a typed React wrapper per plugin is cheaper + safer than designing
// a YAML DSL.

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    ChevronLeft,
    ChevronRight,
    Loader2,
    MoreHorizontal,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

import EmptyState from "../../../../components/EmptyState";
import { palette, space } from "../../../../layout/theme";
import { humanizeError } from "../../../../lib/humanizeError";
import { invokePluginRPC } from "../../../../lib/api/agents/plugins";
import { MetaStrip, useRefreshInterval } from "./MetaStrip";

// ---------------------------------------------------------------------------
// Type surface: props consumed by every plugin's wrapper component.
// ---------------------------------------------------------------------------

/**
 * Form input that operators twiddle to refine the RPC request. The
 * `field` is a string key in the form-state record; `buildRequest`
 * receives the whole record and assembles whatever the plugin's RPC
 * expects (parameter names + nesting are plugin-specific).
 *
 * v1 input kinds (kept intentionally narrow):
 *   - "text"   : free-form string (rare; most ops want search instead)
 *   - "search" : same as text but debounced — fires refetch on idle
 *   - "number" : integer with optional min/max bounds
 *   - "select" : dropdown over a closed set of options
 *   - "toggle" : boolean checkbox
 */
export type RequestFormField =
    | {
          field: string;
          kind: "text";
          label: string;
          default?: string;
          debounceMs?: number;
      }
    | {
          field: string;
          kind: "search";
          label: string;
          default?: string;
          debounceMs?: number;
          placeholder?: string;
      }
    | {
          field: string;
          kind: "number";
          label: string;
          default?: number;
          min?: number;
          max?: number;
          step?: number;
      }
    | {
          field: string;
          kind: "select";
          label: string;
          options: ReadonlyArray<{ value: string; label: string }>;
          default?: string;
      }
    | {
          field: string;
          kind: "toggle";
          label: string;
          default?: boolean;
      };

export interface Column<TRow> {
    /**
     * Default text-rendering source. When `render` is supplied, the
     * field is still used for the React `key` but the rendered
     * content comes from `render(row)`.
     */
    field: keyof TRow & string;
    label: string;
    /**
     * The "name" column — bolder, never truncated. Most tables have
     * exactly one primary column. v1 doesn't enforce that.
     */
    primary?: boolean;
    /** Truncate long cell text with ellipsis instead of wrapping. */
    truncate?: boolean;
    /** Custom cell renderer. Receives the full row. */
    render?: (row: TRow) => ReactNode;
}

/**
 * Pagination behaviour for plugin RPCs that return list slices.
 *
 * Two flavours map to the two pagination styles used by our system
 * plugins (see commits 675d1f7 + f784aab):
 *
 *   - "offset" — sys-net, sys-services, sys-systemd-linux, sys-firewall,
 *                sys-tasks-windows. Request carries `offset` + `limit`;
 *                response carries `totalCount` + `hasMore`. Operators
 *                see "Showing 1–50 of 312" + Prev/Next + page-size knob.
 *
 *   - "cursor" — sys-journald-linux + sys-log-darwin + sys-log-windows.
 *                Request carries `afterCursor` xor `beforeCursor`;
 *                response carries `prevCursor` (oldest) + `nextCursor`
 *                (newest). UI exposes Older / Newer buttons; "Older"
 *                fires `beforeCursor: prevCursor`, "Newer" fires
 *                `afterCursor: nextCursor`.
 *
 * Both flavours reset to the first page whenever the request body
 * changes (the operator typed in the search box, flipped a toggle,
 * etc.) — paging through stale-filter results would be confusing.
 */
export type PaginationConfig =
    | {
          kind: "offset";
          /** Default page size (rows per page). Defaults to 50. */
          pageSize?: number;
          /** Optional dropdown values for the page-size selector. */
          pageSizeOptions?: ReadonlyArray<number>;
          /** Read total count from response. Defaults to `totalCount`. */
          totalCountFrom?: (resp: unknown) => number | undefined;
          /** Read has-more flag from response. Defaults to `hasMore`. */
          hasMoreFrom?: (resp: unknown) => boolean | undefined;
      }
    | {
          kind: "cursor";
          /** Read newer-end cursor. Defaults to `nextCursor`. */
          nextCursorFrom?: (resp: unknown) => string | undefined;
          /** Read older-end cursor. Defaults to `prevCursor`. */
          prevCursorFrom?: (resp: unknown) => string | undefined;
          /** Label for "go to older entries" button. Default "Older". */
          olderLabel?: string;
          /** Label for "go to newer entries" button. Default "Newer". */
          newerLabel?: string;
      };

const DEFAULT_OFFSET_PAGE_SIZE = 50;
const DEFAULT_PAGE_SIZE_OPTIONS = [25, 50, 100, 200] as const;

function readNumberField(resp: unknown, key: string): number | undefined {
    if (resp && typeof resp === "object" && key in resp) {
        const v = (resp as Record<string, unknown>)[key];
        if (typeof v === "number") return v;
    }
    return undefined;
}

function readBoolField(resp: unknown, key: string): boolean | undefined {
    if (resp && typeof resp === "object" && key in resp) {
        const v = (resp as Record<string, unknown>)[key];
        if (typeof v === "boolean") return v;
    }
    return undefined;
}

function readStringField(resp: unknown, key: string): string | undefined {
    if (resp && typeof resp === "object" && key in resp) {
        const v = (resp as Record<string, unknown>)[key];
        if (typeof v === "string" && v !== "") return v;
    }
    return undefined;
}

export interface RowAction<TRow> {
    /** Stable id used for the menu item key. */
    id: string;
    label: string;
    /** RPC method name on the SAME plugin as the table's method. */
    method: string;
    /** Build the action's RPC payload from the row being acted on. */
    args: (row: TRow) => unknown;
    /**
     * When supplied, the action opens an AlertDialog with this text
     * before firing the RPC. The action button label is derived from
     * `label`. Without `confirm`, the RPC fires immediately.
     */
    confirm?: (row: TRow) => string;
    /** Destructive styling on the menu item + dialog action button. */
    danger?: boolean;
}

export interface RPCTableProps<TResponse, TRow> {
    projectID: string;
    agentID: string;
    pluginID: string;
    /** RPC method on the plugin that returns the list payload. */
    method: string;
    /**
     * Operator-tunable request inputs. Defaults populate form state on
     * mount; changes trigger debounced refetch with the new request.
     */
    requestForm?: ReadonlyArray<RequestFormField>;
    /**
     * Build the RPC request object from the current form state.
     * Defaults to the form record verbatim when omitted.
     */
    buildRequest?: (form: Record<string, unknown>) => unknown;
    /** Extract the row array from the RPC response. */
    rowsFrom: (resp: TResponse) => TRow[];
    /** Stable per-row identity (React key + action targeting). */
    /**
     * Stable per-row identity (React key + action targeting). The
     * second argument is the row's index in the resolved list —
     * useful as a tie-breaker when natural keys can collide
     * (journal entries with the same microsecond timestamp, etc.).
     * Most callers ignore it.
     */
    rowKey: (row: TRow, index?: number) => string;
    columns: ReadonlyArray<Column<TRow>>;
    actions?: ReadonlyArray<RowAction<TRow>>;
    /** Auto-refetch interval. 0 / undefined = no polling. */
    refreshMs?: number;
    /**
     * When false, the query is paused — used by the activity-bar
     * pattern to stop polling for offscreen tabs. Defaults to true.
     */
    active?: boolean;
    /** Empty-state body when rowsFrom returns []. */
    emptyText?: string;
    /**
     * Opt-in pagination footer. When set, the table merges `offset`+
     * `limit` (offset mode) or `afterCursor`/`beforeCursor` (cursor
     * mode) into the request, reads pagination metadata off the
     * response, and renders Prev/Next controls below the rows.
     * Resets to page 1 whenever the form-driven request changes.
     */
    pagination?: PaginationConfig;
    /**
     * Show the meta strip (last-updated · manual refresh · interval
     * selector) above the table. Defaults to `true`. Set `false`
     * for narrow embeds where the controls add more noise than
     * value (no current call site).
     */
    metaStrip?: boolean;
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

export default function RPCTable<TResponse, TRow>(
    props: RPCTableProps<TResponse, TRow>,
) {
    const {
        projectID,
        agentID,
        pluginID,
        method,
        requestForm = [],
        buildRequest,
        rowsFrom,
        rowKey,
        columns,
        actions = [],
        refreshMs = 0,
        active = true,
        emptyText,
        pagination,
        metaStrip = true,
    } = props;

    // ----- refresh-interval override (operator-tunable, persisted) -----
    const { effectiveMs: effectiveRefreshMs, chooseInterval: chooseRefreshInterval } =
        useRefreshInterval(pluginID, agentID, refreshMs);

    // ----- form state -----
    const [form, setForm] = useState<Record<string, unknown>>(() =>
        initialFormValues(requestForm),
    );
    const [debouncedForm, setDebouncedForm] = useState(form);
    // Debounce search/text inputs so each keystroke doesn't refire
    // the RPC. The longest debounceMs across registered fields wins
    // (per-field timers would require a richer state machine; v1
    // keeps it simple).
    const debounceMs = useMemo(() => {
        let max = 0;
        for (const f of requestForm) {
            if ("debounceMs" in f && typeof f.debounceMs === "number") {
                if (f.debounceMs > max) max = f.debounceMs;
            }
        }
        return max;
    }, [requestForm]);
    useEffect(() => {
        if (debounceMs <= 0) {
            setDebouncedForm(form);
            return;
        }
        const t = setTimeout(() => setDebouncedForm(form), debounceMs);
        return () => clearTimeout(t);
    }, [form, debounceMs]);

    // ----- pagination state -----
    // Two flavours, see PaginationConfig docs. Each branch carries its
    // own state; only one is active per render. The third branch
    // (no pagination) keeps everything as it was pre-feature.
    const initialPageSize =
        pagination?.kind === "offset"
            ? (pagination.pageSize ?? DEFAULT_OFFSET_PAGE_SIZE)
            : DEFAULT_OFFSET_PAGE_SIZE;
    const [offset, setOffset] = useState(0);
    const [pageSize, setPageSize] = useState(initialPageSize);
    // Cursor mode: null = first page; otherwise we carry whichever
    // direction we last navigated. The response's prev/nextCursor
    // dictates whether the corresponding button stays enabled.
    const [cursorReq, setCursorReq] = useState<
        { afterCursor: string } | { beforeCursor: string } | null
    >(null);

    // Form-driven request body (the plugin-specific bit).
    const baseRequest = useMemo(
        () =>
            buildRequest ? buildRequest(debouncedForm) : { ...debouncedForm },
        [buildRequest, debouncedForm],
    );

    // Reset paging whenever the form changes. Browsing page 7 of a
    // stale filter would be confusing — every form change effectively
    // starts a new dataset.
    const baseRequestKey = useMemo(
        () => JSON.stringify(baseRequest ?? {}),
        [baseRequest],
    );
    useEffect(() => {
        setOffset(0);
        setCursorReq(null);
    }, [baseRequestKey]);

    // ----- RPC query -----
    const requestPayload = useMemo(() => {
        const base =
            baseRequest && typeof baseRequest === "object"
                ? { ...(baseRequest as Record<string, unknown>) }
                : {};
        if (pagination?.kind === "offset") {
            base.offset = offset;
            base.limit = pageSize;
        } else if (pagination?.kind === "cursor" && cursorReq) {
            Object.assign(base, cursorReq);
        }
        return base;
    }, [baseRequest, pagination, offset, pageSize, cursorReq]);

    const queryClient = useQueryClient();
    const queryKey = [
        "plugin-rpc",
        projectID,
        agentID,
        pluginID,
        method,
        requestPayload,
    ] as const;

    const query = useQuery({
        queryKey,
        queryFn: ({ signal }) =>
            invokePluginRPC<TResponse>(
                projectID,
                agentID,
                pluginID,
                method,
                requestPayload,
                { signal },
            ),
        enabled: active && agentID !== "",
        refetchInterval:
            active && effectiveRefreshMs > 0 ? effectiveRefreshMs : false,
        refetchOnWindowFocus: false,
        retry: false,
    });

    // ----- row-action mutation -----
    const [pendingAction, setPendingAction] = useState<{
        action: RowAction<TRow>;
        row: TRow;
    } | null>(null);

    const actionMutation = useMutation({
        mutationFn: async (vars: { action: RowAction<TRow>; row: TRow }) => {
            await invokePluginRPC(
                projectID,
                agentID,
                pluginID,
                vars.action.method,
                vars.action.args(vars.row),
            );
        },
        onSuccess: (_res, vars) => {
            toast.success(`${vars.action.label} ${rowKey(vars.row)}: ok`);
            setPendingAction(null);
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (err) => {
            toast.error(humanizeError(err));
            setPendingAction(null);
        },
    });

    function fireAction(action: RowAction<TRow>, row: TRow) {
        if (action.confirm) {
            setPendingAction({ action, row });
            return;
        }
        actionMutation.mutate({ action, row });
    }

    // ----- render -----
    const rows: TRow[] = useMemo(() => {
        if (!query.data) return [];
        return rowsFrom(query.data);
    }, [query.data, rowsFrom]);

    // ----- pagination metadata extraction -----
    const totalCount =
        pagination?.kind === "offset"
            ? (pagination.totalCountFrom?.(query.data) ??
                  readNumberField(query.data, "totalCount"))
            : undefined;
    const hasMore =
        pagination?.kind === "offset"
            ? (pagination.hasMoreFrom?.(query.data) ??
                  readBoolField(query.data, "hasMore") ??
                  // Fall back to inferring from total + offset + rows.
                  (typeof totalCount === "number"
                      ? offset + rows.length < totalCount
                      : false))
            : false;
    const nextCursor =
        pagination?.kind === "cursor"
            ? (pagination.nextCursorFrom?.(query.data) ??
                  readStringField(query.data, "nextCursor"))
            : undefined;
    const prevCursor =
        pagination?.kind === "cursor"
            ? (pagination.prevCursorFrom?.(query.data) ??
                  readStringField(query.data, "prevCursor"))
            : undefined;

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[3],
            }}
        >
            {requestForm.length > 0 && (
                <RequestForm
                    fields={requestForm}
                    values={form}
                    onChange={setForm}
                />
            )}

            {metaStrip && (
                <MetaStrip
                    dataUpdatedAt={query.dataUpdatedAt}
                    isFetching={query.isFetching}
                    onRefresh={() => void query.refetch()}
                    intervalMs={effectiveRefreshMs}
                    onIntervalChange={chooseRefreshInterval}
                />
            )}

            {query.isLoading ? (
                <div
                    style={{
                        display: "flex",
                        justifyContent: "center",
                        padding: space[6],
                    }}
                >
                    <Loader2 className="size-5 animate-spin" />
                </div>
            ) : query.error ? (
                <EmptyState
                    title="Couldn't load"
                    description={humanizeError(query.error)}
                />
            ) : rows.length === 0 ? (
                <EmptyState
                    title="Nothing to show"
                    description={emptyText ?? "The plugin returned no rows."}
                />
            ) : (
                <Table>
                    <TableHeader>
                        <TableRow>
                            {columns.map((c) => (
                                <TableHead key={c.field}>{c.label}</TableHead>
                            ))}
                            {actions.length > 0 && (
                                <TableHead style={{ width: 48 }} aria-label="Actions">
                                    {""}
                                </TableHead>
                            )}
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {rows.map((row, idx) => {
                            const id = rowKey(row, idx);
                            return (
                                <TableRow key={id}>
                                    {columns.map((c) => (
                                        <TableCell
                                            key={c.field}
                                            data-primary={c.primary ? "true" : "false"}
                                            style={{
                                                fontWeight: c.primary ? 600 : 400,
                                                whiteSpace: c.truncate
                                                    ? "nowrap"
                                                    : undefined,
                                                overflow: c.truncate
                                                    ? "hidden"
                                                    : undefined,
                                                textOverflow: c.truncate
                                                    ? "ellipsis"
                                                    : undefined,
                                                maxWidth: c.truncate ? 320 : undefined,
                                                color: palette.textPrimary,
                                            }}
                                        >
                                            {c.render
                                                ? c.render(row)
                                                : defaultCell(row[c.field])}
                                        </TableCell>
                                    ))}
                                    {actions.length > 0 && (
                                        <TableCell>
                                            <DropdownMenu>
                                                <DropdownMenuTrigger asChild>
                                                    <Button
                                                        variant="ghost"
                                                        size="icon"
                                                        aria-label={`Actions for ${id}`}
                                                    >
                                                        <MoreHorizontal className="size-4" />
                                                    </Button>
                                                </DropdownMenuTrigger>
                                                <DropdownMenuContent align="end">
                                                    {actions.map((a) => (
                                                        <DropdownMenuItem
                                                            key={a.id}
                                                            onClick={() => fireAction(a, row)}
                                                            variant={
                                                                a.danger
                                                                    ? "destructive"
                                                                    : "default"
                                                            }
                                                        >
                                                            {a.label}
                                                        </DropdownMenuItem>
                                                    ))}
                                                </DropdownMenuContent>
                                            </DropdownMenu>
                                        </TableCell>
                                    )}
                                </TableRow>
                            );
                        })}
                    </TableBody>
                </Table>
            )}

            {pagination?.kind === "offset" &&
                !query.isLoading &&
                !query.error && (
                    <OffsetPaginationFooter
                        offset={offset}
                        pageSize={pageSize}
                        rowsOnPage={rows.length}
                        totalCount={totalCount}
                        hasMore={hasMore}
                        pageSizeOptions={
                            pagination.pageSizeOptions ??
                            DEFAULT_PAGE_SIZE_OPTIONS
                        }
                        onPrev={() =>
                            setOffset((o) => Math.max(0, o - pageSize))
                        }
                        onNext={() => setOffset((o) => o + pageSize)}
                        onPageSizeChange={(n) => {
                            setPageSize(n);
                            setOffset(0);
                        }}
                    />
                )}

            {pagination?.kind === "cursor" &&
                !query.isLoading &&
                !query.error &&
                (rows.length > 0 || cursorReq !== null) && (
                    <CursorPaginationFooter
                        olderLabel={pagination.olderLabel ?? "Older"}
                        newerLabel={pagination.newerLabel ?? "Newer"}
                        canGoOlder={Boolean(prevCursor)}
                        canGoNewer={Boolean(nextCursor) || cursorReq !== null}
                        atFirstPage={cursorReq === null}
                        onOlder={() => {
                            if (prevCursor)
                                setCursorReq({ beforeCursor: prevCursor });
                        }}
                        onNewer={() => {
                            if (nextCursor) {
                                setCursorReq({ afterCursor: nextCursor });
                            } else {
                                // No newer entries returned this round —
                                // ratchet back to "live" (latest) view.
                                setCursorReq(null);
                            }
                        }}
                        onLatest={() => setCursorReq(null)}
                    />
                )}

            <AlertDialog
                open={pendingAction !== null}
                onOpenChange={(open) => {
                    if (!open) setPendingAction(null);
                }}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            {pendingAction
                                ? pendingAction.action.label
                                : ""}
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            {pendingAction
                                ? pendingAction.action.confirm?.(pendingAction.row)
                                : ""}
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                if (pendingAction) {
                                    actionMutation.mutate(pendingAction);
                                }
                            }}
                            data-variant={
                                pendingAction?.action.danger ? "destructive" : "default"
                            }
                            style={{
                                backgroundColor: pendingAction?.action.danger
                                    ? palette.danger
                                    : undefined,
                            }}
                        >
                            {pendingAction?.action.label ?? ""}
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function initialFormValues(
    fields: ReadonlyArray<RequestFormField>,
): Record<string, unknown> {
    const out: Record<string, unknown> = {};
    for (const f of fields) {
        switch (f.kind) {
            case "text":
            case "search":
            case "select":
                out[f.field] = f.default ?? "";
                break;
            case "number":
                out[f.field] = f.default ?? 0;
                break;
            case "toggle":
                out[f.field] = f.default ?? false;
                break;
        }
    }
    return out;
}

function defaultCell(v: unknown): ReactNode {
    if (v === null || v === undefined) return "";
    if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") {
        return String(v);
    }
    return JSON.stringify(v);
}

// ---------------------------------------------------------------------------
// Subcomponent: OffsetPaginationFooter
// ---------------------------------------------------------------------------

function OffsetPaginationFooter({
    offset,
    pageSize,
    rowsOnPage,
    totalCount,
    hasMore,
    pageSizeOptions,
    onPrev,
    onNext,
    onPageSizeChange,
}: {
    offset: number;
    pageSize: number;
    rowsOnPage: number;
    totalCount: number | undefined;
    hasMore: boolean;
    pageSizeOptions: ReadonlyArray<number>;
    onPrev: () => void;
    onNext: () => void;
    onPageSizeChange: (n: number) => void;
}) {
    const start = rowsOnPage === 0 ? 0 : offset + 1;
    const end = offset + rowsOnPage;
    const total =
        typeof totalCount === "number"
            ? totalCount
            : // No totalCount in response (older plugin or filter that
              // skips the count) — show "≥ end" as a lower bound.
              undefined;
    const summary =
        rowsOnPage === 0
            ? "No results"
            : total !== undefined
              ? `Showing ${start}–${end} of ${total}`
              : `Showing ${start}–${end}`;

    return (
        <div
            data-testid="rpc-pagination-footer"
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: space[3],
                paddingTop: space[2],
                fontSize: 12,
                color: palette.textMuted,
            }}
        >
            <span>{summary}</span>
            <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                <Label
                    htmlFor="rpc-page-size"
                    style={{ fontSize: 11, color: palette.textMuted }}
                >
                    Page size
                </Label>
                <select
                    id="rpc-page-size"
                    aria-label="Page size"
                    value={String(pageSize)}
                    onChange={(e) => onPageSizeChange(Number(e.target.value))}
                    style={{
                        height: 28,
                        padding: "0 6px",
                        background: palette.surface,
                        color: palette.textPrimary,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 6,
                    }}
                >
                    {pageSizeOptions.map((n) => (
                        <option key={n} value={String(n)}>
                            {n}
                        </option>
                    ))}
                </select>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={onPrev}
                    disabled={offset === 0}
                    aria-label="Previous page"
                >
                    <ChevronLeft className="size-4" />
                </Button>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={onNext}
                    disabled={!hasMore}
                    aria-label="Next page"
                >
                    <ChevronRight className="size-4" />
                </Button>
            </div>
        </div>
    );
}

// ---------------------------------------------------------------------------
// Subcomponent: CursorPaginationFooter
// ---------------------------------------------------------------------------

function CursorPaginationFooter({
    olderLabel,
    newerLabel,
    canGoOlder,
    canGoNewer,
    atFirstPage,
    onOlder,
    onNewer,
    onLatest,
}: {
    olderLabel: string;
    newerLabel: string;
    canGoOlder: boolean;
    canGoNewer: boolean;
    atFirstPage: boolean;
    onOlder: () => void;
    onNewer: () => void;
    onLatest: () => void;
}) {
    return (
        <div
            data-testid="rpc-pagination-footer"
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "flex-end",
                gap: space[2],
                paddingTop: space[2],
                fontSize: 12,
                color: palette.textMuted,
            }}
        >
            {!atFirstPage && (
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={onLatest}
                    aria-label="Jump to latest"
                >
                    Latest
                </Button>
            )}
            <Button
                variant="outline"
                size="sm"
                onClick={onNewer}
                disabled={atFirstPage && !canGoNewer}
                aria-label={newerLabel}
            >
                <ChevronLeft className="size-4" />
                {newerLabel}
            </Button>
            <Button
                variant="outline"
                size="sm"
                onClick={onOlder}
                disabled={!canGoOlder}
                aria-label={olderLabel}
            >
                {olderLabel}
                <ChevronRight className="size-4" />
            </Button>
        </div>
    );
}

// ---------------------------------------------------------------------------
// Subcomponent: RequestForm
// ---------------------------------------------------------------------------

function RequestForm({
    fields,
    values,
    onChange,
}: {
    fields: ReadonlyArray<RequestFormField>;
    values: Record<string, unknown>;
    onChange: (next: Record<string, unknown>) => void;
}) {
    const setOne = (field: string, value: unknown) =>
        onChange({ ...values, [field]: value });

    return (
        <div
            style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "flex-end",
                gap: space[3],
                paddingBottom: space[1],
            }}
        >
            {fields.map((f) => (
                <div
                    key={f.field}
                    style={{ display: "flex", flexDirection: "column", gap: 4 }}
                >
                    <Label
                        htmlFor={`rpc-form-${f.field}`}
                        style={{ fontSize: 11, color: palette.textMuted }}
                    >
                        {f.label}
                    </Label>
                    {renderInput(f, values, setOne)}
                </div>
            ))}
        </div>
    );
}

function renderInput(
    f: RequestFormField,
    values: Record<string, unknown>,
    setOne: (field: string, value: unknown) => void,
): ReactNode {
    const id = `rpc-form-${f.field}`;
    switch (f.kind) {
        case "text":
        case "search":
            return (
                <Input
                    id={id}
                    type={f.kind === "search" ? "search" : "text"}
                    placeholder={f.kind === "search" ? f.placeholder : undefined}
                    value={(values[f.field] as string) ?? ""}
                    onChange={(e) => setOne(f.field, e.target.value)}
                    style={{ width: 240 }}
                />
            );
        case "number":
            return (
                <Input
                    id={id}
                    type="number"
                    min={f.min}
                    max={f.max}
                    step={f.step}
                    value={(values[f.field] as number) ?? 0}
                    onChange={(e) => setOne(f.field, Number(e.target.value))}
                    style={{ width: 120 }}
                />
            );
        case "select":
            // shadcn's <Select> uses Radix portal which is finicky in
            // jsdom; native <select> matches userEvent.selectOptions
            // reliably and renders identically inside our theme.
            return (
                <select
                    id={id}
                    aria-label={f.label}
                    value={(values[f.field] as string) ?? ""}
                    onChange={(e) => setOne(f.field, e.target.value)}
                    style={{
                        height: 32,
                        padding: "0 8px",
                        background: palette.surface,
                        color: palette.textPrimary,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 6,
                    }}
                >
                    {f.options.map((o) => (
                        <option key={o.value} value={o.value}>
                            {o.label}
                        </option>
                    ))}
                </select>
            );
        case "toggle":
            return (
                <input
                    id={id}
                    type="checkbox"
                    checked={Boolean(values[f.field])}
                    onChange={(e) => setOne(f.field, e.target.checked)}
                    style={{ width: 16, height: 16 }}
                />
            );
    }
}
