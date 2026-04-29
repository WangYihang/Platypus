import * as React from "react";
import { Fragment, useMemo } from "react";
import {
    flexRender,
    getCoreRowModel,
    useReactTable,
    type ColumnDef,
    type ColumnSizingState,
    type SortingState,
} from "@tanstack/react-table";

import { usePreference } from "../../../lib/preferences";
import { useDraggable, useDroppable, useDndMonitor } from "@dnd-kit/core";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import dayjs from "dayjs";

import { isHiddenEntry, pickFileIcon } from "./fileIcons";
import EntryTooltip from "./EntryTooltip";
import { joinPath } from "./paths";

import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/cn";

import type { FileEntryDTO } from "../../../platform/App.web";
import type { Density } from "./useDensity";
import { formatMode } from "./paths";
import { humanize } from "../../../lib/format";

interface Props {
    entries: FileEntryDTO[];
    currentPath: string;
    selectedNames: Set<string>;
    setSelectedNames: (names: Set<string>) => void;
    onOpen: (entry: FileEntryDTO) => void;
    sorting: SortingState;
    setSorting: (s: SortingState) => void;
    // Called when the user drops an entry onto a droppable directory row
    // or breadcrumb segment. We surface it here so the parent can issue
    // the RenameFile RPC.
    onInternalMove?: (fromEntry: FileEntryDTO, toDir: string) => void;
    // Optional row wrapper for cross-cutting concerns the table
    // shouldn't have to know about (today: right-click context menu).
    // The function receives the entry and the rendered <tr> and
    // returns a node — usually the <tr> wrapped in a primitive that
    // attaches its own listeners via Radix's `asChild` pattern.
    wrapRow?: (entry: FileEntryDTO, node: React.ReactNode) => React.ReactNode;
    // Row-height density. "comfortable" preserves the historical
    // padding; "compact" tightens cell padding + drops the font size
    // a step so a denser screen fits more rows without scroll.
    density?: Density;
}

// DraggableRow wires dnd-kit's drag source to a single entry row. We
// keep the drag id equal to the entry name so DndMonitor in the parent
// can reconstruct the (from, to) pair without a custom data payload.
//
// `forwardRef` + `...rest` spread is load-bearing: per-row right-click
// menus wrap each <DraggableRow> via `<ContextMenuTrigger asChild>`,
// which clones the child to attach an `onContextMenu` listener and a
// ref. Without these the contextmenu event bubbles up to the outer
// empty-area trigger and the user sees the New file / Upload here
// menu instead of the row's Open / Download / Rename / Delete menu.
interface DraggableRowProps extends Omit<React.HTMLAttributes<HTMLTableRowElement>, "onSelect"> {
    entry: FileEntryDTO;
    isSelected: boolean;
    onSelectChange: (e: React.MouseEvent) => void;
    onOpen: () => void;
    isDroppable: boolean;
    children: React.ReactNode;
}

const DraggableRow = React.forwardRef<HTMLTableRowElement, DraggableRowProps>(function DraggableRow(
    {
        entry,
        isSelected,
        onSelectChange,
        onOpen,
        isDroppable,
        children,
        onClick: extraOnClick,
        onDoubleClick: extraOnDoubleClick,
        ...rest
    },
    forwardedRef,
) {
    const drag = useDraggable({
        id: `row:${entry.name}`,
        data: { entry },
    });
    const drop = useDroppable({
        id: `drop:${entry.name}`,
        data: { dirName: entry.name, isDir: entry.isDir },
        disabled: !isDroppable,
    });

    const setRefs = (node: HTMLTableRowElement | null) => {
        drag.setNodeRef(node);
        if (isDroppable) drop.setNodeRef(node);
        if (typeof forwardedRef === "function") forwardedRef(node);
        else if (forwardedRef) forwardedRef.current = node;
    };

    return (
        <TableRow
            ref={setRefs}
            data-selected={isSelected || undefined}
            className={cn(
                "cursor-pointer select-none",
                isSelected && "bg-accent",
                drop.isOver && isDroppable && "outline outline-2 outline-primary",
                drag.isDragging && "opacity-60",
            )}
            onClick={(e) => {
                onSelectChange(e);
                extraOnClick?.(e);
            }}
            onDoubleClick={(e) => {
                onOpen();
                extraOnDoubleClick?.(e);
            }}
            {...drag.listeners}
            {...drag.attributes}
            {...rest}
        >
            {children}
        </TableRow>
    );
});

export default function FileTable({
    entries,
    currentPath,
    selectedNames,
    setSelectedNames,
    onOpen,
    sorting,
    setSorting,
    onInternalMove,
    wrapRow,
    density = "comfortable",
}: Props) {
    useDndMonitor({
        onDragEnd(event) {
            if (!onInternalMove) return;
            const from = event.active.data.current?.entry as FileEntryDTO | undefined;
            const toDir = event.over?.data.current?.dirName as string | undefined;
            const toIsDir = event.over?.data.current?.isDir as boolean | undefined;
            if (!from || !toDir || !toIsDir) return;
            if (from.name === toDir) return;
            onInternalMove(from, toDir);
        },
    });

    const columns = useMemo<ColumnDef<FileEntryDTO>[]>(
        () => [
            {
                id: "select",
                header: () => <span className="sr-only">Select</span>,
                cell: () => null, // checkbox is rendered inline below so the click target includes the row
                enableSorting: false,
                size: 36,
            },
            {
                accessorKey: "name",
                header: "Name",
                cell: ({ row }) => {
                    const e = row.original;
                    const { Icon, color } = pickFileIcon(e);
                    const dim = isHiddenEntry(e);
                    return (
                        <EntryTooltip
                            entry={e}
                            fullPath={joinPath(currentPath, e.name)}
                            side="right"
                        >
                            <div
                                className={cn(
                                    "flex items-center gap-2 font-mono",
                                    dim && "opacity-60",
                                )}
                            >
                                <Icon className={cn("size-4", color)} />
                                <span>{e.name}</span>
                                {e.isSymlink && e.symlinkTarget && (
                                    <span className="text-xs text-muted-foreground">→ {e.symlinkTarget}</span>
                                )}
                            </div>
                        </EntryTooltip>
                    );
                },
            },
            {
                accessorKey: "size",
                header: "Size",
                cell: ({ row }) => (row.original.isDir ? "—" : humanize(row.original.size)),
            },
            {
                accessorKey: "mode",
                header: "Mode",
                cell: ({ row }) => (
                    <span className="font-mono text-xs">
                        {formatMode(row.original.mode, row.original.isDir, row.original.isSymlink)}
                    </span>
                ),
            },
            {
                accessorKey: "modTimeUnix",
                header: "Modified",
                cell: ({ row }) => {
                    const ns = row.original.modTimeUnix;
                    if (!ns) return "—";
                    return (
                        <span className="text-xs text-muted-foreground">
                            {dayjs(ns / 1_000_000).format("YYYY-MM-DD HH:mm")}
                        </span>
                    );
                },
            },
        ],
        // currentPath is included so the EntryTooltip closure
        // captures the live directory after a cd. Without this the
        // tooltip would still render paths relative to the directory
        // the table was first mounted at.
        [currentPath],
    );

    // Sorting is applied at the parent (FileBrowser) so the same key
    // reorders both the list and the grid view. We still pass the
    // sort state into the table so the column header renders the
    // correct asc/desc indicator, but skip getSortedRowModel to avoid
    // double-sorting (and to support sort ids — e.g. "type" — that
    // don't have a corresponding column accessor).
    // Column-sizing is fully managed by us so the user's choices
    // survive cd / refetch / reload. We mirror tanstack's local
    // state into the typed preference registry so the same widths
    // apply across host pages and tabs.
    const [persistedSizes, setPersistedSizes] = usePreference("ui.files.columnWidths");
    const [columnSizing, setColumnSizing] = React.useState<ColumnSizingState>(
        () => persistedSizes,
    );
    React.useEffect(() => {
        // Push live changes into the prefs registry. Cheap shallow
        // equality keeps us from writing on every drag tick — the
        // preference event listener would re-fire and re-render.
        const a = JSON.stringify(persistedSizes);
        const b = JSON.stringify(columnSizing);
        if (a !== b) setPersistedSizes(columnSizing);
    }, [columnSizing, persistedSizes, setPersistedSizes]);

    const table = useReactTable({
        data: entries,
        columns,
        state: { sorting, columnSizing },
        onSortingChange: (updater) => {
            const next = typeof updater === "function" ? updater(sorting) : updater;
            setSorting(next);
        },
        onColumnSizingChange: setColumnSizing,
        enableColumnResizing: true,
        columnResizeMode: "onChange",
        getCoreRowModel: getCoreRowModel(),
        manualSorting: true,
    });

    function toggleRow(entry: FileEntryDTO, ev: React.MouseEvent) {
        const next = new Set(selectedNames);
        if (ev.shiftKey || ev.metaKey || ev.ctrlKey) {
            if (next.has(entry.name)) next.delete(entry.name);
            else next.add(entry.name);
        } else {
            next.clear();
            next.add(entry.name);
        }
        setSelectedNames(next);
    }

    return (
        <Table
            data-density={density}
            className={cn(
                density === "compact" &&
                    // Override the shadcn defaults: TableHead is `h-10` (40px)
                    // and TableCell is `p-2` (8px) — both designed for cards
                    // with a few rows. The file browser routinely shows
                    // hundreds of entries, so collapse the row height by
                    // tightening padding and clamping <th> height. The
                    // 2026-04 density pass tightens further: header h-6,
                    // 11px caps; rows py-0.5 + leading-tight + 12px text
                    // so a row lands at ~22 px.
                    "[&_th]:h-6 [&_th]:py-0 [&_th]:text-[11px] [&_td]:py-0.5 [&_td]:text-[12px] [&_td]:leading-tight",
            )}
        >
            <TableHeader className="sticky top-0 z-10 bg-background [&_tr]:border-b">
                {table.getHeaderGroups().map((hg) => (
                    <TableRow key={hg.id}>
                        {hg.headers.map((header) => {
                            const canSort = header.column.getCanSort();
                            const canResize = header.column.getCanResize();
                            const sortDir = header.column.getIsSorted();
                            return (
                                <TableHead
                                    key={header.id}
                                    style={{ width: header.getSize() || undefined }}
                                    className={cn(
                                        "relative bg-background",
                                        canSort && "select-none",
                                    )}
                                >
                                    <div
                                        onClick={
                                            canSort
                                                ? header.column.getToggleSortingHandler()
                                                : undefined
                                        }
                                        className={cn(
                                            "flex items-center gap-1",
                                            canSort && "cursor-pointer",
                                        )}
                                    >
                                        {flexRender(header.column.columnDef.header, header.getContext())}
                                        {canSort &&
                                            (sortDir === "asc" ? (
                                                <ArrowUp className="size-3" />
                                            ) : sortDir === "desc" ? (
                                                <ArrowDown className="size-3" />
                                            ) : (
                                                <ArrowUpDown className="size-3 opacity-40" />
                                            ))}
                                    </div>
                                    {canResize && (
                                        // The resize handle has to sit on top of
                                        // the sort click target, so we use a
                                        // small absolute strip on the right edge
                                        // of the header. mousedown/touchstart go
                                        // straight to tanstack's resize tracker;
                                        // the click event is stopped so the
                                        // sort handler upstream doesn't fire on
                                        // the same gesture.
                                        <div
                                            onMouseDown={header.getResizeHandler()}
                                            onTouchStart={header.getResizeHandler()}
                                            onClick={(e) => e.stopPropagation()}
                                            role="separator"
                                            aria-orientation="vertical"
                                            aria-label="Resize column"
                                            className={cn(
                                                "absolute right-0 top-0 h-full w-1 cursor-col-resize select-none touch-none",
                                                "opacity-0 hover:opacity-100",
                                                header.column.getIsResizing()
                                                    ? "bg-primary opacity-100"
                                                    : "bg-border",
                                            )}
                                        />
                                    )}
                                </TableHead>
                            );
                        })}
                    </TableRow>
                ))}
            </TableHeader>
            <TableBody>
                {table.getRowModel().rows.map((row) => {
                    const e = row.original;
                    const selected = selectedNames.has(e.name);
                    const rowNode = (
                        <DraggableRow
                            key={e.name}
                            entry={e}
                            isSelected={selected}
                            isDroppable={e.isDir}
                            onSelectChange={(ev) => toggleRow(e, ev)}
                            onOpen={() => onOpen(e)}
                        >
                            {row.getVisibleCells().map((cell) => {
                                if (cell.column.id === "select") {
                                    return (
                                        <TableCell key={cell.id}>
                                            <Checkbox
                                                checked={selected}
                                                onCheckedChange={() => {
                                                    const next = new Set(selectedNames);
                                                    if (selected) next.delete(e.name);
                                                    else next.add(e.name);
                                                    setSelectedNames(next);
                                                }}
                                                onClick={(ev) => ev.stopPropagation()}
                                            />
                                        </TableCell>
                                    );
                                }
                                return (
                                    <TableCell key={cell.id}>
                                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                                    </TableCell>
                                );
                            })}
                        </DraggableRow>
                    );
                    return wrapRow ? (
                        <Fragment key={e.name}>{wrapRow(e, rowNode)}</Fragment>
                    ) : (
                        rowNode
                    );
                })}
                {/* Empty / filter-narrowed-empty state lives in the
                    parent <FileBrowser> so it can offer CTAs. We
                    just render no rows. */}
            </TableBody>
        </Table>
    );
}
