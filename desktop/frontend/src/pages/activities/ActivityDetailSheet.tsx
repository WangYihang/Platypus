import * as React from "react";

import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { palette } from "../../layout/theme";
import { ActivityItem } from "../../lib/api";
import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";

import { OUTCOME_TONE, formatDuration } from "./format";

interface Props {
    item: ActivityItem | null;
    onOpenChange: (open: boolean) => void;
}

// Right-side sheet showing the full structured record for a single
// activity row. Kept separate so the table stays compact and the sheet
// can grow richer over time (links to session, request_id
// cross-reference, etc.).
export default function ActivityDetailSheet({ item, onOpenChange }: Props) {
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
        if (item.session_id)
            rows.push({ label: "Session", value: <Mono>{item.session_id}</Mono> });
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
                            <Row key={r.label} label={r.label} value={r.value} />
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

function Row({ label, value }: { label: string; value: React.ReactNode }) {
    return (
        <>
            <div className="pt-0.5 text-[11px] uppercase text-text-muted">{label}</div>
            <div className="text-text-primary break-words">{value}</div>
        </>
    );
}
