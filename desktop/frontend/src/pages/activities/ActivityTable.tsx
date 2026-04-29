import { Loader2 } from "lucide-react";

import Card from "../../components/Card";
import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { palette, space } from "../../layout/theme";
import { ActivityItem } from "../../lib/api";
import { fromNow } from "../../lib/time";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

import { OUTCOME_TONE, formatDuration } from "./format";

interface Props {
    items: ActivityItem[];
    nextCursor: string | null;
    loadingMore: boolean;
    onLoadMore: () => void;
    onSelect: (item: ActivityItem) => void;
}

export default function ActivityTable({
    items,
    nextCursor,
    loadingMore,
    onLoadMore,
    onSelect,
}: Props) {
    return (
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
                            onClick={() => onSelect(it)}
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
                                <StatusPill tone={OUTCOME_TONE[it.outcome] ?? "neutral"}>
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
                        onClick={onLoadMore}
                    >
                        {loadingMore && <Loader2 className="size-3.5 animate-spin" />}
                        Load more
                    </Button>
                </div>
            )}
        </Card>
    );
}

// Truncated, tooltip-backed target label so long paths don't blow up rows.
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
