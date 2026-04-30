import { useMemo, useState } from "react";
import { BookOpen, ChevronDown, ChevronRight, Loader2, Play } from "lucide-react";

import { Button } from "@/components/ui/button";

import Mono from "../../../components/Mono";
import { palette, radius, space } from "../../../layout/theme";
import type { AuditorResult, AvailableAuditor } from "../../../lib/api";

interface Props {
    auditors: AvailableAuditor[];
    /** Most-recent per-auditor lifecycle, indexed by auditor id. Used
     *  to decorate the row with last-run status + leak count. */
    lastResults: Map<string, AuditorResult>;
    /** Set of auditor ids currently re-running (one per partial). */
    runningSet: Set<string>;
    /** True when any audit (full or partial) is in flight, used to
     *  disable per-row buttons during a full re-audit. */
    isAnyRunning: boolean;
    onRerun: (auditorID: string) => void;
}

// CoveragePanel mirrors the security tab's "What do we check?" panel.
// It enumerates every registered Auditor so the operator sees the
// scanner's capability boundary up-front (rather than inferring it
// from whatever leaks happen to surface today). Collapsed by default
// to keep the tab tight; the per-row "Run" button triggers a partial
// audit so an operator who just rotated their AWS keys can verify the
// dotfiles auditor in isolation without re-running the slow webapp
// directory walk.
export function CoveragePanel({
    auditors,
    lastResults,
    runningSet,
    isAnyRunning,
    onRerun,
}: Props) {
    const [open, setOpen] = useState(false);

    const grouped = useMemo(() => {
        const map = new Map<string, AvailableAuditor[]>();
        for (const a of auditors) {
            const arr = map.get(a.category) ?? [];
            arr.push(a);
            map.set(a.category, arr);
        }
        return Array.from(map.entries())
            .sort((a, b) => a[0].localeCompare(b[0]))
            .map(
                ([cat, items]) =>
                    [
                        cat,
                        items.slice().sort((a, b) => a.id.localeCompare(b.id)),
                    ] as const,
            );
    }, [auditors]);

    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                background: palette.surface,
                overflow: "hidden",
            }}
        >
            <button
                type="button"
                onClick={() => setOpen((v) => !v)}
                style={{
                    all: "unset",
                    cursor: "pointer",
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[2]}px ${space[3]}px`,
                    width: "100%",
                    boxSizing: "border-box",
                }}
                aria-expanded={open}
            >
                {open ? (
                    <ChevronDown className="size-3.5" />
                ) : (
                    <ChevronRight className="size-3.5" />
                )}
                <BookOpen className="size-3.5" style={{ color: palette.info }} />
                <strong style={{ fontSize: 13, color: palette.textPrimary }}>
                    What does this audit cover?
                </strong>
                <span style={{ fontSize: 12, color: palette.textSecondary }}>
                    {auditors.length} auditor{auditors.length === 1 ? "" : "s"}
                </span>
                <span style={{ flex: 1 }} />
                <span style={{ fontSize: 12, color: palette.textMuted }}>
                    {open ? "hide" : "show"}
                </span>
            </button>
            {open && (
                <div
                    style={{
                        padding: space[3],
                        borderTop: `1px solid ${palette.border}`,
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                    }}
                >
                    <p style={{ margin: 0, fontSize: 12, color: palette.textSecondary }}>
                        Each auditor enumerates a specific source on the host —
                        process environment, shell history, well-known
                        credential dotfiles, database / web app config files.
                        Detection of credential-shaped strings is delegated to
                        the gitleaks ruleset; we handle the "where to look".
                        Secrets are redacted on the agent before leaving it.
                    </p>
                    {grouped.map(([cat, items]) => (
                        <CategorySection
                            key={cat}
                            category={cat}
                            items={items}
                            lastResults={lastResults}
                            runningSet={runningSet}
                            isAnyRunning={isAnyRunning}
                            onRerun={onRerun}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

function CategorySection({
    category,
    items,
    lastResults,
    runningSet,
    isAnyRunning,
    onRerun,
}: {
    category: string;
    items: AvailableAuditor[];
    lastResults: Map<string, AuditorResult>;
    runningSet: Set<string>;
    isAnyRunning: boolean;
    onRerun: (id: string) => void;
}) {
    return (
        <div>
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    fontWeight: 600,
                    textTransform: "uppercase",
                    letterSpacing: 0.5,
                    marginBottom: 4,
                }}
            >
                {category}
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                {items.map((a) => (
                    <AuditorRow
                        key={a.id}
                        auditor={a}
                        last={lastResults.get(a.id)}
                        running={runningSet.has(a.id)}
                        isAnyRunning={isAnyRunning}
                        onRerun={() => onRerun(a.id)}
                    />
                ))}
            </div>
        </div>
    );
}

function AuditorRow({
    auditor,
    last,
    running,
    isAnyRunning,
    onRerun,
}: {
    auditor: AvailableAuditor;
    last: AuditorResult | undefined;
    running: boolean;
    isAnyRunning: boolean;
    onRerun: () => void;
}) {
    const dim = !auditor.applicable;
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: `${space[1]}px ${space[2]}px`,
                borderRadius: radius.sm,
                opacity: dim ? 0.55 : 1,
            }}
        >
            <Mono
                style={{
                    fontSize: 12,
                    color: palette.textPrimary,
                    minWidth: 160,
                }}
            >
                {auditor.id}
            </Mono>
            <span
                style={{
                    fontSize: 12,
                    color: palette.textSecondary,
                    flex: 1,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                }}
                title={auditor.description}
            >
                {auditor.title || auditor.description || "—"}
            </span>
            <StatusBadge last={last} running={running} applicable={auditor.applicable} />
            <Button
                type="button"
                size="xs"
                variant="ghost"
                onClick={onRerun}
                disabled={isAnyRunning || !auditor.applicable}
                title={
                    auditor.applicable
                        ? `Re-run ${auditor.id}`
                        : "Auditor not applicable on this host"
                }
            >
                {running ? (
                    <Loader2 className="size-3 animate-spin" />
                ) : (
                    <Play className="size-3" />
                )}
            </Button>
        </div>
    );
}

function StatusBadge({
    last,
    running,
    applicable,
}: {
    last: AuditorResult | undefined;
    running: boolean;
    applicable: boolean;
}) {
    if (running) {
        return (
            <span style={{ fontSize: 11, color: palette.info }}>auditing…</span>
        );
    }
    if (!applicable) {
        return (
            <span style={{ fontSize: 11, color: palette.textMuted }}>
                not applicable
            </span>
        );
    }
    if (!last) {
        return (
            <span style={{ fontSize: 11, color: palette.textMuted }}>not run</span>
        );
    }
    if (last.status === "error") {
        return (
            <span
                style={{ fontSize: 11, color: palette.danger }}
                title={last.error}
            >
                error
            </span>
        );
    }
    if (last.status === "skipped") {
        return (
            <span style={{ fontSize: 11, color: palette.textMuted }}>skipped</span>
        );
    }
    if (last.leak_count > 0) {
        return (
            <span style={{ fontSize: 11, color: palette.warning }}>
                {last.leak_count} leak{last.leak_count === 1 ? "" : "s"}
            </span>
        );
    }
    return <span style={{ fontSize: 11, color: palette.success }}>clean</span>;
}

export default CoveragePanel;
