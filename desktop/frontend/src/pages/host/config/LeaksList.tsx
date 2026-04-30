import { useMemo, useState } from "react";

import { palette, radius, space } from "../../../layout/theme";
import type { ConfigLeak, Risk } from "../../../lib/api";

import LeakRow from "./LeakRow";
import { compareRisk, RISKS, riskTone } from "./riskTone";

interface Props {
    leaks: ConfigLeak[];
}

// LeaksList renders the leak rows grouped by category, sorted within
// each group by risk descending. A small filter bar above the list
// lets the operator narrow by risk and free-text search across title
// and location — categories are shown as group headers so a category
// filter would be redundant.
export function LeaksList({ leaks }: Props) {
    const [riskFilter, setRiskFilter] = useState<Set<Risk>>(new Set());
    const [q, setQ] = useState("");

    const filtered = useMemo(() => {
        const needle = q.trim().toLowerCase();
        return leaks.filter((l) => {
            if (riskFilter.size > 0 && !riskFilter.has(l.risk)) return false;
            if (needle) {
                const hay = (l.title + " " + l.location + " " + l.pattern).toLowerCase();
                if (!hay.includes(needle)) return false;
            }
            return true;
        });
    }, [leaks, riskFilter, q]);

    const grouped = useMemo(() => {
        const map = new Map<string, ConfigLeak[]>();
        for (const l of filtered) {
            const arr = map.get(l.category) ?? [];
            arr.push(l);
            map.set(l.category, arr);
        }
        return Array.from(map.entries())
            .sort((a, b) => a[0].localeCompare(b[0]))
            .map(
                ([cat, items]) =>
                    [
                        cat,
                        items.slice().sort((a, b) => compareRisk(a.risk, b.risk)),
                    ] as const,
            );
    }, [filtered]);

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <FilterBar
                riskFilter={riskFilter}
                onToggleRisk={(r) =>
                    setRiskFilter((prev) => {
                        const next = new Set(prev);
                        if (next.has(r)) next.delete(r);
                        else next.add(r);
                        return next;
                    })
                }
                q={q}
                onQChange={setQ}
                total={leaks.length}
                shown={filtered.length}
            />
            {grouped.length === 0 ? (
                <div
                    style={{
                        padding: space[4],
                        textAlign: "center",
                        fontSize: 13,
                        color: palette.textMuted,
                        border: `1px dashed ${palette.border}`,
                        borderRadius: radius.md,
                    }}
                >
                    {leaks.length === 0
                        ? "No leaks detected on this host."
                        : "No leaks match the current filter."}
                </div>
            ) : (
                grouped.map(([cat, items]) => (
                    <CategoryGroup key={cat} category={cat} items={items} />
                ))
            )}
        </div>
    );
}

function CategoryGroup({
    category,
    items,
}: {
    category: string;
    items: ConfigLeak[];
}) {
    return (
        <div>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    marginBottom: space[1],
                }}
            >
                <span
                    style={{
                        fontSize: 11,
                        color: palette.textMuted,
                        fontWeight: 600,
                        textTransform: "uppercase",
                        letterSpacing: 0.5,
                    }}
                >
                    {category}
                </span>
                <span style={{ fontSize: 11, color: palette.textMuted }}>
                    {items.length}
                </span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                {items.map((l) => (
                    <LeakRow key={l.id} leak={l} />
                ))}
            </div>
        </div>
    );
}

function FilterBar({
    riskFilter,
    onToggleRisk,
    q,
    onQChange,
    total,
    shown,
}: {
    riskFilter: Set<Risk>;
    onToggleRisk: (r: Risk) => void;
    q: string;
    onQChange: (s: string) => void;
    total: number;
    shown: number;
}) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                flexWrap: "wrap",
            }}
        >
            <div style={{ display: "inline-flex", gap: 4 }}>
                {RISKS.map((r) => {
                    const active = riskFilter.has(r);
                    const tone = riskTone(r);
                    return (
                        <button
                            key={r}
                            type="button"
                            onClick={() => onToggleRisk(r)}
                            style={{
                                all: "unset",
                                cursor: "pointer",
                                padding: "2px 8px",
                                fontSize: 11,
                                fontWeight: 600,
                                textTransform: "uppercase",
                                letterSpacing: 0.4,
                                color: active ? tone.fg : palette.textMuted,
                                background: active ? tone.bg : "transparent",
                                border: `1px solid ${active ? tone.fg : palette.border}`,
                                borderRadius: 999,
                                lineHeight: 1.4,
                            }}
                            aria-pressed={active}
                        >
                            {tone.label}
                        </button>
                    );
                })}
            </div>
            <input
                type="search"
                value={q}
                onChange={(e) => onQChange(e.target.value)}
                placeholder="Filter by title, location, pattern…"
                style={{
                    flex: 1,
                    minWidth: 200,
                    background: palette.surface,
                    color: palette.textPrimary,
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.sm,
                    padding: `${space[1]}px ${space[2]}px`,
                    fontSize: 12,
                }}
            />
            <span style={{ fontSize: 11, color: palette.textMuted }}>
                {shown === total ? `${total} leak${total === 1 ? "" : "s"}` : `${shown} / ${total}`}
            </span>
        </div>
    );
}

export default LeaksList;
