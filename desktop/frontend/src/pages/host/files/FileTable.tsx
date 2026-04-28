import { Fragment, useMemo } from "react";
import {
    flexRender,
    getCoreRowModel,
    getSortedRowModel,
    useReactTable,
    type ColumnDef,
    type SortingState,
} from "@tanstack/react-table";
import { useDraggable, useDroppable, useDndMonitor } from "@dnd-kit/core";
import { ArrowDown, ArrowUp, ArrowUpDown, File, FileSymlink, Folder } from "lucide-react";
import dayjs from "dayjs";

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
}

// DraggableRow wires dnd-kit's drag source to a single entry row. We
// keep the drag id equal to the entry name so DndMonitor in the parent
// can reconstruct the (from, to) pair without a custom data payload.
function DraggableRow({
    entry,
    isSelected,
    onSelectChange,
    onOpen,
    isDroppable,
    children,
}: {
    entry: FileEntryDTO;
    isSelected: boolean;
    onSelectChange: (e: React.MouseEvent) => void;
    onOpen: () => void;
    isDroppable: boolean;
    children: React.ReactNode;
}) {
    const drag = useDraggable({
        id: `row:${entry.name}`,
        data: { entry },
    });
    const drop = useDroppable({
        id: `drop:${entry.name}`,
        data: { dirName: entry.name, isDir: entry.isDir },
        disabled: !isDroppable,
    });

    const setRefs = (node: HTMLElement | null) => {
        drag.setNodeRef(node);
        if (isDroppable) drop.setNodeRef(node);
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
            onClick={onSelectChange}
            onDoubleClick={onOpen}
            {...drag.listeners}
            {...drag.attributes}
        >
            {children}
        </TableRow>
    );
}

export default function FileTable({
    entries,
    selectedNames,
    setSelectedNames,
    onOpen,
    sorting,
    setSorting,
    onInternalMove,
    wrapRow,
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
                    return (
                        <div className="flex items-center gap-2 font-mono">
                            {e.isDir ? (
                                <Folder className="size-4 text-amber-500" />
                            ) : e.isSymlink ? (
                                <FileSymlink className="size-4 text-sky-500" />
                            ) : (
                                <File className="size-4 text-muted-foreground" />
                            )}
                            <span>{e.name}</span>
                            {e.isSymlink && e.symlinkTarget && (
                                <span className="text-xs text-muted-foreground">→ {e.symlinkTarget}</span>
                            )}
                        </div>
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
        [],
    );

    const table = useReactTable({
        data: entries,
        columns,
        state: { sorting },
        onSortingChange: (updater) => {
            const next = typeof updater === "function" ? updater(sorting) : updater;
            setSorting(next);
        },
        getCoreRowModel: getCoreRowModel(),
        getSortedRowModel: getSortedRowModel(),
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
        <Table>
            <TableHeader>
                {table.getHeaderGroups().map((hg) => (
                    <TableRow key={hg.id}>
                        {hg.headers.map((header) => {
                            const canSort = header.column.getCanSort();
                            const sortDir = header.column.getIsSorted();
                            return (
                                <TableHead
                                    key={header.id}
                                    style={{ width: header.getSize() || undefined }}
                                    onClick={canSort ? header.column.getToggleSortingHandler() : undefined}
                                    className={canSort ? "cursor-pointer select-none" : ""}
                                >
                                    <div className="flex items-center gap-1">
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
                {entries.length === 0 && (
                    <TableRow>
                        <TableCell colSpan={columns.length} className="py-10 text-center text-sm text-muted-foreground">
                            Empty directory
                        </TableCell>
                    </TableRow>
                )}
            </TableBody>
        </Table>
    );
}
