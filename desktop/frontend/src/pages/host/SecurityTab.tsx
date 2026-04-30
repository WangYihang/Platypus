import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Shield } from "lucide-react";
import { useTranslation } from "react-i18next";

import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import RefreshButton from "../../components/RefreshButton";
import { palette, radius, space } from "../../layout/theme";
import {
    HostSecurityScan,
    SecurityCheckResult,
    SecurityFinding,
    Severity,
    SeverityCounts,
    getHostSecurityScan,
    rescanHost,
} from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";
import { fromNow } from "../../lib/time";

import { severityTone } from "../fleet/cards/SecurityBadge";

import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";

interface Props {
    projectID: string;
    hostID: string;
    active: boolean;
}

const SEVERITIES: Severity[] = ["critical", "high", "medium", "low", "info"];

// SecurityTab loads the host's most recent persisted scan from the
// server-side cache and lets the operator trigger a fresh scan.
//
// Behaviour differences from ProcessesTab:
//   · No interval polling — a scan is heavy (file walks, /proc reads)
//     and findings don't drift on their own. Refresh is explicit.
//   · The empty state branches on data === null (server returned 404
//     because the host has never been scanned) vs data with empty
//     findings (scanned, all clean). Same well-known shape difference
//     the storage layer pins, surfaced here so the user understands
//     why no findings are showing.
//   · Re-scan is a mutation that writes through to the cache plus
//     invalidates the hosts list (HostCard severity badge) and the
//     project-level findings table.
export default function SecurityTab({ projectID, hostID, active }: Props) {
    const { t } = useTranslation("security");
    const queryClient = useQueryClient();

    const {
        data,
        isFetching,
        error,
        refetch,
    } = useQuery({
        queryKey: qk.hostSecurityScan(projectID, hostID),
        queryFn: () => getHostSecurityScan(projectID, hostID),
        enabled: active,
        refetchOnWindowFocus: false,
    });

    const rescan = useMutation({
        mutationFn: () => rescanHost(projectID, hostID),
        onSuccess: (fresh) => {
            queryClient.setQueryData(qk.hostSecurityScan(projectID, hostID), fresh);
            queryClient.invalidateQueries({
                queryKey: qk.hostSecurityScans(projectID, hostID, 10),
            });
            queryClient.invalidateQueries({ queryKey: qk.hosts(projectID) });
            // Project-level findings page caches per-options; nuke
            // the whole shelf so any open page rerenders.
            queryClient.invalidateQueries({
                queryKey: ["projectSecurityFindings", projectID],
            });
        },
    });

    const [severityFilter, setSeverityFilter] = useState<Set<Severity>>(new Set());
    const [categoryFilter, setCategoryFilter] = useState<Set<string>>(new Set());

    const findings = data?.findings ?? [];
    const checks = data?.checks ?? [];
    const counts: SeverityCounts =
        data?.severity_counts ?? { critical: 0, high: 0, medium: 0, low: 0, info: 0 };

    const categoriesPresent = useMemo(() => {
        const s = new Set<string>();
        for (const f of findings) s.add(f.category);
        return Array.from(s).sort();
    }, [findings]);

    const filtered = useMemo(() => {
        return findings.filter((f) => {
            if (severityFilter.size > 0 && !severityFilter.has(f.severity)) return false;
            if (categoryFilter.size > 0 && !categoryFilter.has(f.category)) return false;
            return true;
        });
    }, [findings, severityFilter, categoryFilter]);

    const skipped = useMemo(
        () => checks.filter((c) => c.status !== "ok"),
        [checks],
    );

    const isPending = rescan.isPending;
    const loading = isFetching || isPending;

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <Header
                data={data}
                counts={counts}
                loading={loading}
                isPending={isPending}
                onRescan={() => rescan.mutate()}
                onRefresh={() => void refetch()}
            />

            {error && (
                <Alert kind="danger">{humanizeError(error)}</Alert>
            )}
            {rescan.error && (
                <Alert kind="warning">
                    {t("errors.scanFailed")} — {humanizeError(rescan.error)}
                </Alert>
            )}

            {data === null && !isPending ? (
                <EmptyState
                    icon={<Shield />}
                    title={t("neverScanned")}
                    description={t("neverScannedHint")}
                    action={
                        <Button
                            type="button"
                            size="sm"
                            onClick={() => rescan.mutate()}
                            disabled={isPending}
                        >
                            {isPending ? t("rescan.running") : t("rescan.first")}
                        </Button>
                    }
                />
            ) : null}

            {data && findings.length === 0 && !isPending && (
                <EmptyState
                    icon={<Shield style={{ color: palette.success }} />}
                    title={t("empty.title")}
                    description={t("empty.subtitle")}
                />
            )}

            {data && findings.length > 0 && (
                <>
                    <FilterRow
                        countsPresent={SEVERITIES.filter(
                            (s) => counts[s] > 0,
                        )}
                        categoriesPresent={categoriesPresent}
                        severity={severityFilter}
                        category={categoryFilter}
                        onSeverityToggle={(s) => {
                            setSeverityFilter((prev) => toggle(prev, s));
                        }}
                        onCategoryToggle={(c) => {
                            setCategoryFilter((prev) => toggle(prev, c));
                        }}
                        onClear={() => {
                            setSeverityFilter(new Set());
                            setCategoryFilter(new Set());
                        }}
                        counts={counts}
                    />
                    <FindingsTable findings={filtered} />
                </>
            )}

            {skipped.length > 0 && <SkippedChecks checks={skipped} />}
        </div>
    );
}

function Header({
    data,
    counts,
    loading,
    isPending,
    onRescan,
    onRefresh,
}: {
    data: HostSecurityScan | null | undefined;
    counts: SeverityCounts;
    loading: boolean;
    isPending: boolean;
    onRescan: () => void;
    onRefresh: () => void;
}) {
    const { t } = useTranslation("security");
    const scannedAt =
        data?.started_at_unix ? new Date(data.started_at_unix * 1000) : null;

    return (
        <div
            style={{
                display: "flex",
                gap: space[3],
                alignItems: "center",
                flexWrap: "wrap",
            }}
        >
            <SeverityChips counts={counts} />
            <span style={{ flex: 1 }} />
            <span style={{ fontSize: 12, color: palette.textSecondary }}>
                {scannedAt ? t("lastScanned", { when: fromNow(scannedAt) }) : "—"}
            </span>
            {data && (
                <RefreshButton loading={loading && !isPending} onClick={onRefresh} />
            )}
            {data && (
                <Button
                    type="button"
                    size="sm"
                    variant="default"
                    onClick={onRescan}
                    disabled={isPending}
                >
                    {isPending ? t("rescan.running") : t("rescan.button")}
                </Button>
            )}
        </div>
    );
}

function SeverityChips({ counts }: { counts: SeverityCounts }) {
    const { t } = useTranslation("security");
    return (
        <div style={{ display: "inline-flex", gap: space[2], flexWrap: "wrap" }}>
            {SEVERITIES.map((s) => {
                const n = counts[s];
                const tone = severityTone(s);
                const muted = n === 0;
                return (
                    <span
                        key={s}
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[1],
                            padding: `2px ${space[2]}px`,
                            borderRadius: radius.pill,
                            background: muted ? "transparent" : tone.background,
                            color: muted ? palette.textMuted : tone.foreground,
                            border: `1px solid ${
                                muted ? palette.border : tone.border
                            }`,
                            fontSize: 12,
                            fontWeight: muted ? 500 : 600,
                        }}
                    >
                        {t(`severity.${s}`)} {n}
                    </span>
                );
            })}
        </div>
    );
}

function FilterRow({
    countsPresent,
    categoriesPresent,
    severity,
    category,
    onSeverityToggle,
    onCategoryToggle,
    onClear,
    counts,
}: {
    countsPresent: Severity[];
    categoriesPresent: string[];
    severity: Set<Severity>;
    category: Set<string>;
    onSeverityToggle: (s: Severity) => void;
    onCategoryToggle: (c: string) => void;
    onClear: () => void;
    counts: SeverityCounts;
}) {
    const { t } = useTranslation("security");
    if (countsPresent.length === 0 && categoriesPresent.length === 0) return null;
    const hasFilter = severity.size > 0 || category.size > 0;
    return (
        <div
            style={{
                display: "flex",
                gap: space[3],
                alignItems: "center",
                flexWrap: "wrap",
                fontSize: 12,
                color: palette.textSecondary,
            }}
        >
            {countsPresent.length > 0 && (
                <FilterGroup label={t("filter.severity")}>
                    {countsPresent.map((s) => (
                        <Toggle
                            key={s}
                            active={severity.has(s)}
                            onClick={() => onSeverityToggle(s)}
                            tone={severityTone(s)}
                        >
                            {t(`severity.${s}`)} {counts[s]}
                        </Toggle>
                    ))}
                </FilterGroup>
            )}
            {categoriesPresent.length > 0 && (
                <FilterGroup label={t("filter.category")}>
                    {categoriesPresent.map((c) => (
                        <Toggle
                            key={c}
                            active={category.has(c)}
                            onClick={() => onCategoryToggle(c)}
                        >
                            {translateCategory(t, c)}
                        </Toggle>
                    ))}
                </FilterGroup>
            )}
            {hasFilter && (
                <Button type="button" size="sm" variant="ghost" onClick={onClear}>
                    {t("filter.clear")}
                </Button>
            )}
        </div>
    );
}

function FilterGroup({
    label,
    children,
}: {
    label: string;
    children: React.ReactNode;
}) {
    return (
        <div style={{ display: "inline-flex", gap: space[2], alignItems: "center" }}>
            <span>{label}:</span>
            <span style={{ display: "inline-flex", gap: space[1], flexWrap: "wrap" }}>
                {children}
            </span>
        </div>
    );
}

function Toggle({
    active,
    onClick,
    children,
    tone,
}: {
    active: boolean;
    onClick: () => void;
    children: React.ReactNode;
    tone?: ReturnType<typeof severityTone>;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            style={{
                all: "unset",
                cursor: "pointer",
                padding: `2px ${space[2]}px`,
                borderRadius: radius.pill,
                background: active
                    ? tone?.background ?? palette.surfaceHover
                    : "transparent",
                color: active
                    ? tone?.foreground ?? palette.textPrimary
                    : palette.textSecondary,
                border: `1px solid ${
                    active ? tone?.border ?? palette.borderStrong : palette.border
                }`,
                fontWeight: active ? 600 : 500,
                fontSize: 11,
            }}
        >
            {children}
        </button>
    );
}

function FindingsTable({ findings }: { findings: SecurityFinding[] }) {
    const { t } = useTranslation("security");
    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead className="w-[110px]">{t("table.severity")}</TableHead>
                    <TableHead>{t("table.title")}</TableHead>
                    <TableHead className="w-[110px]">{t("table.category")}</TableHead>
                    <TableHead className="w-[140px]">{t("table.check")}</TableHead>
                </TableRow>
            </TableHeader>
            <TableBody>
                {findings.map((f) => (
                    <FindingRow key={f.id} f={f} />
                ))}
            </TableBody>
        </Table>
    );
}

function FindingRow({ f }: { f: SecurityFinding }) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);
    const tone = severityTone(f.severity);
    return (
        <>
            <TableRow
                onClick={() => setOpen((v) => !v)}
                style={{ cursor: "pointer" }}
                aria-expanded={open}
            >
                <TableCell>
                    <span
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[1],
                            padding: `2px ${space[2]}px`,
                            borderRadius: radius.pill,
                            background: tone.background,
                            color: tone.foreground,
                            border: `1px solid ${tone.border}`,
                            fontSize: 11,
                            fontWeight: 600,
                            textTransform: "capitalize",
                        }}
                    >
                        {t(`severity.${f.severity}`)}
                    </span>
                </TableCell>
                <TableCell>
                    <span
                        style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}
                    >
                        {open ? (
                            <ChevronDown className="size-3.5" />
                        ) : (
                            <ChevronRight className="size-3.5" />
                        )}
                        <span>{f.title}</span>
                    </span>
                </TableCell>
                <TableCell>
                    <span style={{ fontSize: 12, color: palette.textSecondary }}>
                        {translateCategory(t, f.category)}
                    </span>
                </TableCell>
                <TableCell>
                    <Mono size={11}>{f.check_id || f.finding_id}</Mono>
                </TableCell>
            </TableRow>
            {open && (
                <TableRow>
                    <TableCell colSpan={4} style={{ background: palette.surfaceHover }}>
                        <FindingDetails f={f} />
                    </TableCell>
                </TableRow>
            )}
        </>
    );
}

function FindingDetails({ f }: { f: SecurityFinding }) {
    const { t } = useTranslation("security");
    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[2],
                padding: `${space[2]}px 0`,
                fontSize: 13,
            }}
        >
            <DetailBlock label={t("details.description")} body={f.description} />
            <DetailBlock label={t("details.evidence")} body={f.evidence} mono />
            <DetailBlock label={t("details.remediation")} body={f.remediation} />
            {f.references && f.references.length > 0 && (
                <div>
                    <strong style={{ color: palette.textSecondary, fontSize: 12 }}>
                        {t("details.references")}:
                    </strong>{" "}
                    {f.references.map((r, i) => (
                        <span key={`${r}-${i}`}>
                            {i > 0 ? ", " : ""}
                            {linkifyReference(r)}
                        </span>
                    ))}
                </div>
            )}
        </div>
    );
}

function DetailBlock({
    label,
    body,
    mono,
}: {
    label: string;
    body: string;
    mono?: boolean;
}) {
    if (!body) return null;
    return (
        <div>
            <strong style={{ color: palette.textSecondary, fontSize: 12 }}>
                {label}:
            </strong>{" "}
            {mono ? (
                <Mono size={12}>{body}</Mono>
            ) : (
                <span style={{ whiteSpace: "pre-wrap" }}>{body}</span>
            )}
        </div>
    );
}

function SkippedChecks({ checks }: { checks: SecurityCheckResult[] }) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);
    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                background: palette.surface,
                padding: `${space[2]}px ${space[3]}px`,
                fontSize: 12,
            }}
        >
            <button
                type="button"
                onClick={() => setOpen((v) => !v)}
                style={{
                    all: "unset",
                    cursor: "pointer",
                    display: "inline-flex",
                    alignItems: "center",
                    gap: space[1],
                    color: palette.textSecondary,
                    fontWeight: 600,
                }}
                aria-expanded={open}
            >
                {open ? (
                    <ChevronDown className="size-3.5" />
                ) : (
                    <ChevronRight className="size-3.5" />
                )}
                {t("checks.skippedHeader", { count: checks.length })}
            </button>
            {open && (
                <div style={{ marginTop: space[2], display: "flex", flexDirection: "column", gap: space[1] }}>
                    <p style={{ color: palette.textMuted, margin: 0 }}>
                        {t("checks.skippedHint")}
                    </p>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[200px]">id</TableHead>
                                <TableHead className="w-[100px]">status</TableHead>
                                <TableHead>error</TableHead>
                                <TableHead className="w-[100px]">elapsed</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {checks.map((c) => (
                                <TableRow key={c.id}>
                                    <TableCell>
                                        <Mono size={11}>{c.id}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <span style={{ color: c.status === "error" ? palette.danger : palette.textSecondary }}>
                                            {t(`checks.status${capitalize(c.status)}`, { defaultValue: c.status })}
                                        </span>
                                    </TableCell>
                                    <TableCell>
                                        <Mono size={11}>{c.error || "—"}</Mono>
                                    </TableCell>
                                    <TableCell>{c.elapsed_ms}ms</TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
            )}
        </div>
    );
}

function Alert({
    kind,
    children,
}: {
    kind: "danger" | "warning";
    children: React.ReactNode;
}) {
    const color = kind === "danger" ? palette.danger : palette.warning;
    return (
        <div
            style={{
                padding: `${space[2]}px ${space[3]}px`,
                border: `1px solid ${color}`,
                borderRadius: radius.md,
                color,
                fontSize: 12,
            }}
        >
            {children}
        </div>
    );
}

function toggle<T>(set: Set<T>, value: T): Set<T> {
    const next = new Set(set);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    return next;
}

function capitalize(s: string) {
    return s.charAt(0).toUpperCase() + s.slice(1);
}

// translateCategory falls back to the literal category id when no
// translation is registered. Lets the agent register new categories
// without forcing an i18n PR before the UI displays them.
//
// The `t` function from i18next accepts a `defaultValue` option so
// missing keys land on the literal id rather than the dotted key.
function translateCategory(
    t: (key: string, options?: { defaultValue?: string }) => string,
    category: string,
) {
    return t(`category.${category}`, { defaultValue: category });
}

const cveRe = /^CVE-\d{4}-\d+$/;
function linkifyReference(ref: string) {
    if (cveRe.test(ref)) {
        const url = `https://nvd.nist.gov/vuln/detail/${ref}`;
        return (
            <a
                href={url}
                target="_blank"
                rel="noreferrer"
                style={{ color: palette.info }}
            >
                {ref}
            </a>
        );
    }
    return <span>{ref}</span>;
}
