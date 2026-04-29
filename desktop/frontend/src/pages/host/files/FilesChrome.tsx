import {
    ArrowDown,
    ArrowUp,
    ArrowUpDown,
    ChevronUp,
    Eye,
    EyeOff,
    LayoutGrid,
    LayoutList,
    Map as MapIcon,
    Rows2,
    Rows3,
    SlidersHorizontal,
} from "lucide-react";
import type { SortingState } from "@tanstack/react-table";

import RefreshButton from "../../../components/RefreshButton";
import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuCheckboxItem,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "@/components/ui/popover";

import Crumb from "./Crumb";
import { quickPathsForHost } from "./quickPaths";
import type { FileSortId } from "./sortEntries";

interface Props {
    crumbs: Array<{ path: string; label: string }>;
    canGoUp: boolean;
    onGoUp: () => void;
    onCd: (path: string) => void;
    onRefresh: () => void;
    refreshLoading: boolean;
    statusText: string;
    quickPaths: ReturnType<typeof quickPathsForHost>;
    viewMode: "list" | "grid";
    setViewMode: (m: "list" | "grid") => void;
    density: "compact" | "comfortable";
    setDensity: (d: "compact" | "comfortable") => void;
    showHidden: boolean;
    setShowHidden: (next: boolean) => void;
    sorting: SortingState;
    setSorting: (next: SortingState) => void;
}

interface SortOption {
    id: FileSortId;
    label: string;
    // Direction-axis labels used in the menu so "Size" reads as
    // "Smallest first" / "Largest first" instead of an abstract
    // ascending / descending. This is the same convention Finder /
    // Files apps use and removes a tiny cognitive bump for the user.
    ascLabel: string;
    descLabel: string;
}

const SORT_OPTIONS: SortOption[] = [
    { id: "name", label: "Name", ascLabel: "A → Z", descLabel: "Z → A" },
    { id: "size", label: "Size", ascLabel: "Smallest first", descLabel: "Largest first" },
    { id: "modTimeUnix", label: "Modified", ascLabel: "Oldest first", descLabel: "Newest first" },
    { id: "type", label: "Type", ascLabel: "Folders first", descLabel: "Files first" },
];

export default function FilesChrome({
    crumbs,
    canGoUp,
    onGoUp,
    onCd,
    onRefresh,
    refreshLoading,
    statusText,
    quickPaths,
    viewMode,
    setViewMode,
    density,
    setDensity,
    showHidden,
    setShowHidden,
    sorting,
    setSorting,
}: Props) {
    const head = sorting[0];
    const activeSortId = (head?.id as FileSortId | undefined) ?? "name";
    const activeDesc = !!head?.desc;
    const activeOption = SORT_OPTIONS.find((o) => o.id === activeSortId) ?? SORT_OPTIONS[0];

    function chooseSort(id: FileSortId) {
        // Click an inactive field → asc; click the active field → flip
        // direction. Mirrors the column-header semantics in FileTable.
        if (activeSortId === id) {
            setSorting([{ id, desc: !activeDesc }]);
        } else {
            setSorting([{ id, desc: false }]);
        }
    }

    return (
        <div data-testid="files-chrome" className="flex items-center gap-2">
            <div
                data-testid="files-breadcrumb-row"
                className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
            >
                <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={onGoUp}
                    disabled={!canGoUp}
                    title="Up"
                >
                    <ChevronUp className="size-3.5" />
                </Button>
                <RefreshButton
                    variant="ghost"
                    iconOnly
                    loading={refreshLoading}
                    onClick={onRefresh}
                    aria-label="Refresh"
                    title="Refresh"
                    data-testid="files-refresh"
                />
                {crumbs.map((c, idx) => {
                    // suppress the "/" separator when previous crumb is already "/"
                    const showSep = idx > 0 && crumbs[idx - 1].label !== "/";
                    return (
                        <div key={c.path} className="flex items-center gap-1">
                            {showSep && (
                                <span className="text-muted-foreground">/</span>
                            )}
                            <Crumb
                                path={c.path}
                                label={c.label}
                                onClick={() => onCd(c.path)}
                                isLast={idx === crumbs.length - 1}
                            />
                        </div>
                    );
                })}
            </div>

            <span
                data-testid="files-status"
                className="hidden whitespace-nowrap text-[11px] text-muted-foreground sm:inline"
            >
                {statusText}
            </span>

            {quickPaths && quickPaths.length > 0 && (
                <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                        <Button
                            type="button"
                            size="icon-sm"
                            variant="ghost"
                            aria-label="Go to common path"
                            title="Go to common path"
                            data-testid="files-goto"
                        >
                            <MapIcon className="size-3.5" />
                        </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="min-w-[180px]">
                        <DropdownMenuLabel>Go to</DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        {quickPaths.map((p) => (
                            <DropdownMenuItem
                                key={p.path}
                                onSelect={() => onCd(p.path)}
                                title={p.title}
                                className="font-mono text-xs"
                            >
                                {p.label}
                                <span className="ml-auto text-[10px] text-muted-foreground">
                                    {p.path}
                                </span>
                            </DropdownMenuItem>
                        ))}
                    </DropdownMenuContent>
                </DropdownMenu>
            )}

            {/* Sort menu: shared between list + grid views via the
                lifted `sorting` state. The list view's column headers
                still toggle the same key so both surfaces stay in
                sync. */}
            <DropdownMenu>
                <DropdownMenuTrigger asChild>
                    <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label={`Sort by ${activeOption.label}`}
                        title={`Sort: ${activeOption.label} (${
                            activeDesc ? activeOption.descLabel : activeOption.ascLabel
                        })`}
                        data-testid="files-sort"
                    >
                        {activeDesc ? (
                            <ArrowDown className="size-3.5" />
                        ) : head ? (
                            <ArrowUp className="size-3.5" />
                        ) : (
                            <ArrowUpDown className="size-3.5" />
                        )}
                    </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-[180px]">
                    <DropdownMenuLabel>Sort by</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    {SORT_OPTIONS.map((opt) => {
                        const active = activeSortId === opt.id;
                        return (
                            <DropdownMenuItem
                                key={opt.id}
                                onSelect={() => chooseSort(opt.id)}
                                className="text-xs"
                            >
                                <span className={active ? "font-medium" : undefined}>
                                    {opt.label}
                                </span>
                                {active && (
                                    <span className="ml-auto inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                                        {activeDesc ? opt.descLabel : opt.ascLabel}
                                        {activeDesc ? (
                                            <ArrowDown className="size-3" />
                                        ) : (
                                            <ArrowUp className="size-3" />
                                        )}
                                    </span>
                                )}
                            </DropdownMenuItem>
                        );
                    })}
                    <DropdownMenuSeparator />
                    <DropdownMenuCheckboxItem
                        checked={activeDesc}
                        onCheckedChange={(checked) =>
                            setSorting([{ id: activeSortId, desc: !!checked }])
                        }
                        className="text-xs"
                    >
                        Reverse order
                    </DropdownMenuCheckboxItem>
                </DropdownMenuContent>
            </DropdownMenu>

            {/* Quick toggle: show / hide dotfiles. Promoted out of the
                view-options popover because operators flip this often
                enough (config dirs, .git, .ssh, …) that hiding it
                under a second click hurts. */}
            <Button
                type="button"
                size="icon-sm"
                variant={showHidden ? "secondary" : "ghost"}
                aria-label={showHidden ? "Hide hidden files" : "Show hidden files"}
                aria-pressed={showHidden}
                title={showHidden ? "Hide hidden files" : "Show hidden files"}
                onClick={() => setShowHidden(!showHidden)}
                data-testid="files-toggle-hidden"
            >
                {showHidden ? (
                    <Eye className="size-3.5" />
                ) : (
                    <EyeOff className="size-3.5" />
                )}
            </Button>

            {/* List ↔ grid toggle now lives in the toolbar itself
                (was tucked inside the sliders popover) so it's
                reachable in one click. */}
            <div
                className="flex items-center rounded-md border"
                data-testid="files-view-toggle"
            >
                <Button
                    type="button"
                    size="icon-sm"
                    variant={viewMode === "list" ? "secondary" : "ghost"}
                    aria-label="List view"
                    aria-pressed={viewMode === "list"}
                    title="List view"
                    onClick={() => setViewMode("list")}
                >
                    <LayoutList className="size-3.5" />
                </Button>
                <Button
                    type="button"
                    size="icon-sm"
                    variant={viewMode === "grid" ? "secondary" : "ghost"}
                    aria-label="Grid view"
                    aria-pressed={viewMode === "grid"}
                    title="Grid view"
                    onClick={() => setViewMode("grid")}
                >
                    <LayoutGrid className="size-3.5" />
                </Button>
            </div>

            {/* Density still sits behind the sliders popover — it's a
                set-and-forget preference rather than something the
                user toggles per-task. */}
            <Popover>
                <PopoverTrigger asChild>
                    <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label="View options"
                        title="View options"
                        data-testid="files-view"
                    >
                        <SlidersHorizontal className="size-3.5" />
                    </Button>
                </PopoverTrigger>
                <PopoverContent
                    align="end"
                    className="w-auto min-w-[200px] p-3"
                >
                    <div className="flex flex-col gap-3 text-xs">
                        <div className="flex flex-col gap-1.5">
                            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                                Density
                            </span>
                            <div className="flex items-center rounded-md border">
                                <Button
                                    type="button"
                                    size="icon-sm"
                                    variant={density === "compact" ? "secondary" : "ghost"}
                                    aria-label="Compact density"
                                    aria-pressed={density === "compact"}
                                    onClick={() => setDensity("compact")}
                                    title="Compact rows"
                                >
                                    <Rows3 className="size-3.5" />
                                </Button>
                                <Button
                                    type="button"
                                    size="icon-sm"
                                    variant={density === "comfortable" ? "secondary" : "ghost"}
                                    aria-label="Comfortable density"
                                    aria-pressed={density === "comfortable"}
                                    onClick={() => setDensity("comfortable")}
                                    title="Comfortable rows"
                                >
                                    <Rows2 className="size-3.5" />
                                </Button>
                            </div>
                        </div>
                    </div>
                </PopoverContent>
            </Popover>
        </div>
    );
}
