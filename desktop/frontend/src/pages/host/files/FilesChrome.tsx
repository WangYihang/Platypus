import {
    ChevronUp,
    LayoutGrid,
    LayoutList,
    Map as MapIcon,
    Rows2,
    Rows3,
    SlidersHorizontal,
} from "lucide-react";

import RefreshButton from "../../../components/RefreshButton";
import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
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
}

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
}: Props) {
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
                        <div className="flex flex-col gap-1.5">
                            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                                Layout
                            </span>
                            <div className="flex items-center rounded-md border">
                                <Button
                                    type="button"
                                    size="icon-sm"
                                    variant={viewMode === "list" ? "secondary" : "ghost"}
                                    aria-label="List view"
                                    aria-pressed={viewMode === "list"}
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
                                    onClick={() => setViewMode("grid")}
                                >
                                    <LayoutGrid className="size-3.5" />
                                </Button>
                            </div>
                        </div>
                    </div>
                </PopoverContent>
            </Popover>
        </div>
    );
}
