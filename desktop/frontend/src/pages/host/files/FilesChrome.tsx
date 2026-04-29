import { useEffect, useRef, useState } from "react";
import {
    ArrowDown,
    ArrowLeft,
    ArrowRight,
    ArrowUp,
    ArrowUpDown,
    ChevronUp,
    Eye,
    EyeOff,
    LayoutGrid,
    LayoutList,
    Map as MapIcon,
    Pencil,
    Rows2,
    Rows3,
    Search,
    SlidersHorizontal,
    Trash2,
    X,
} from "lucide-react";
import type { SortingState } from "@tanstack/react-table";

import RefreshButton from "../../../components/RefreshButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
    currentPath: string;
    canGoUp: boolean;
    onGoUp: () => void;
    canBack: boolean;
    canForward: boolean;
    onBack: () => void;
    onForward: () => void;
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
    foldersFirst: boolean;
    setFoldersFirst: (next: boolean) => void;
    useTrash: boolean;
    setUseTrash: (next: boolean) => void;
    sorting: SortingState;
    setSorting: (next: SortingState) => void;
    filter: string;
    setFilter: (s: string) => void;
    // Counter-style signals from parent: when they tick the chrome
    // pops the corresponding affordance (path input / filter focus).
    // Counters rather than booleans so re-pressing the shortcut
    // re-triggers even if the chrome is already in that state.
    pathInputOpenSignal: number;
    filterFocusSignal: number;
}

interface SortOption {
    id: FileSortId;
    label: string;
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
    currentPath,
    canGoUp,
    onGoUp,
    canBack,
    canForward,
    onBack,
    onForward,
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
    foldersFirst,
    setFoldersFirst,
    useTrash,
    setUseTrash,
    sorting,
    setSorting,
    filter,
    setFilter,
    pathInputOpenSignal,
    filterFocusSignal,
}: Props) {
    const head = sorting[0];
    const activeSortId = (head?.id as FileSortId | undefined) ?? "name";
    const activeDesc = !!head?.desc;
    const activeOption = SORT_OPTIONS.find((o) => o.id === activeSortId) ?? SORT_OPTIONS[0];

    // Path-input mode: replaces the breadcrumb with an editable input
    // (Cmd+L / click-the-pencil). Submitting cd's; Escape cancels.
    const [pathInputOpen, setPathInputOpen] = useState(false);
    const [pathInputValue, setPathInputValue] = useState(currentPath);
    const pathInputRef = useRef<HTMLInputElement | null>(null);

    // Open + select the current path each time the parent ticks the
    // signal — Cmd+L and the pencil button both do this.
    useEffect(() => {
        if (pathInputOpenSignal === 0) return;
        setPathInputOpen(true);
        setPathInputValue(currentPath);
        // Defer focus until React commits the input to the DOM.
        queueMicrotask(() => {
            pathInputRef.current?.focus();
            pathInputRef.current?.select();
        });
    }, [pathInputOpenSignal, currentPath]);

    // Sync the input with the live path when it's not being edited
    // (the user clicks a breadcrumb or hits Back, the input should
    // reflect the new location next time it opens).
    useEffect(() => {
        if (!pathInputOpen) setPathInputValue(currentPath);
    }, [currentPath, pathInputOpen]);

    // Filter focus signal: pop focus into the search field on Cmd+F.
    const filterRef = useRef<HTMLInputElement | null>(null);
    useEffect(() => {
        if (filterFocusSignal === 0) return;
        queueMicrotask(() => {
            filterRef.current?.focus();
            filterRef.current?.select();
        });
    }, [filterFocusSignal]);

    function chooseSort(id: FileSortId) {
        if (activeSortId === id) {
            setSorting([{ id, desc: !activeDesc }]);
        } else {
            setSorting([{ id, desc: false }]);
        }
    }

    function commitPath() {
        const trimmed = pathInputValue.trim();
        if (!trimmed) {
            setPathInputOpen(false);
            return;
        }
        // Normalise: collapse double-slashes, strip trailing slash
        // unless the path *is* the root.
        const normalised = trimmed.replace(/\/{2,}/g, "/").replace(/\/$/, "") || "/";
        onCd(normalised);
        setPathInputOpen(false);
    }

    return (
        <div data-testid="files-chrome" className="flex items-center gap-2">
            <div
                data-testid="files-breadcrumb-row"
                className="flex min-w-0 flex-1 items-center gap-1"
            >
                <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={onBack}
                    disabled={!canBack}
                    aria-label="Back"
                    title="Back (Alt+←)"
                    data-testid="files-back"
                >
                    <ArrowLeft className="size-3.5" />
                </Button>
                <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={onForward}
                    disabled={!canForward}
                    aria-label="Forward"
                    title="Forward (Alt+→)"
                    data-testid="files-forward"
                >
                    <ArrowRight className="size-3.5" />
                </Button>
                <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={onGoUp}
                    disabled={!canGoUp}
                    title="Up (Backspace)"
                >
                    <ChevronUp className="size-3.5" />
                </Button>
                <RefreshButton
                    variant="ghost"
                    iconOnly
                    loading={refreshLoading}
                    onClick={onRefresh}
                    aria-label="Refresh"
                    title="Refresh (F5)"
                    data-testid="files-refresh"
                />

                {pathInputOpen ? (
                    <form
                        className="flex min-w-0 flex-1 items-center gap-1"
                        onSubmit={(e) => {
                            e.preventDefault();
                            commitPath();
                        }}
                    >
                        <Input
                            ref={pathInputRef}
                            value={pathInputValue}
                            onChange={(e) => setPathInputValue(e.target.value)}
                            onBlur={() => setPathInputOpen(false)}
                            onKeyDown={(e) => {
                                if (e.key === "Escape") {
                                    e.preventDefault();
                                    setPathInputOpen(false);
                                }
                            }}
                            placeholder="/path/to/dir"
                            aria-label="Go to path"
                            className="h-7 min-w-0 flex-1 font-mono text-xs"
                            data-testid="files-path-input"
                        />
                    </form>
                ) : (
                    <div
                        className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
                    >
                        {crumbs.map((c, idx) => {
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
                        <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Edit path"
                            title="Edit path (Ctrl/Cmd+L)"
                            onClick={() => {
                                setPathInputValue(currentPath);
                                setPathInputOpen(true);
                                queueMicrotask(() => {
                                    pathInputRef.current?.focus();
                                    pathInputRef.current?.select();
                                });
                            }}
                            data-testid="files-edit-path"
                        >
                            <Pencil className="size-3" />
                        </Button>
                    </div>
                )}
            </div>

            {/* Inline filter — narrows the visible listing by a
                case-insensitive substring of the entry name. Cmd+F
                pops focus into the field via filterFocusSignal. */}
            <div className="relative hidden md:block">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
                <Input
                    ref={filterRef}
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Escape") setFilter("");
                    }}
                    placeholder="Filter…"
                    aria-label="Filter files"
                    className="h-7 w-40 pl-7 pr-6 text-xs"
                    data-testid="files-filter"
                />
                {filter && (
                    <button
                        type="button"
                        onClick={() => setFilter("")}
                        aria-label="Clear filter"
                        className="absolute right-1 top-1/2 -translate-y-1/2 rounded p-0.5 text-muted-foreground hover:bg-accent"
                    >
                        <X className="size-3" />
                    </button>
                )}
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
                <DropdownMenuContent align="end" className="min-w-[200px]">
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
                    <DropdownMenuCheckboxItem
                        checked={foldersFirst}
                        onCheckedChange={(checked) => setFoldersFirst(!!checked)}
                        className="text-xs"
                    >
                        Folders first
                    </DropdownMenuCheckboxItem>
                </DropdownMenuContent>
            </DropdownMenu>

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
                    className="w-auto min-w-[220px] p-3"
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

                        <div className="flex flex-col gap-1.5">
                            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                                Behaviour
                            </span>
                            <label className="flex items-start gap-2 leading-tight">
                                <input
                                    type="checkbox"
                                    checked={useTrash}
                                    onChange={(e) => setUseTrash(e.target.checked)}
                                    className="mt-0.5"
                                />
                                <span className="flex-1">
                                    <span className="inline-flex items-center gap-1 font-medium">
                                        <Trash2 className="size-3" />
                                        Move to Trash on delete
                                    </span>
                                    <span className="block text-[10px] text-muted-foreground">
                                        Renames into /tmp/.platypus-trash instead of unlinking. Cleared on host reboot.
                                    </span>
                                </span>
                            </label>
                        </div>
                    </div>
                </PopoverContent>
            </Popover>
        </div>
    );
}
