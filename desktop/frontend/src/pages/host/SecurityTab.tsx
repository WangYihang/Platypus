import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    AlertTriangle,
    BookOpen,
    CheckCircle2,
    ChevronDown,
    ChevronRight,
    Loader2,
    MinusCircle,
    RefreshCw,
    Shield,
    XCircle,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import { palette, radius, space } from "../../layout/theme";
import {
    AvailableCheck,
    HostSecurityScan,
    SecurityCheckResult,
    SecurityFinding,
    Severity,
    SeverityCounts,
    getHostSecurityScan,
    listAvailableChecks,
    rescanHost,
} from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { qk } from "../../lib/queryKeys";
import { fromNow } from "../../lib/time";

import { severityTone } from "../fleet/cards/SecurityBadge";

import { Button } from "@/components/ui/button";

interface Props {
    projectID: string;
    hostID: string;
    active: boolean;
}

const SEVERITIES: Severity[] = ["critical", "high", "medium", "low", "info"];

// Per-row status drives the icon + label on each checklist entry.
//   not_run        — never persisted + not currently scanning
//   running        — mutation in flight covering this check
//   pass           — last run was ok with zero findings
//   findings       — last run was ok with N findings
//   skipped        — last run reported the check as not applicable
//   error          — last run errored (agent could not read the data)
//   not_applicable — Applicable() = false at enumeration time
type RowStatus =
    | "not_run"
    | "running"
    | "pass"
    | "findings"
    | "skipped"
    | "error"
    | "not_applicable";

interface RowModel {
    id: string;
    category: string;
    applicable: boolean;
    status: RowStatus;
    elapsedMs?: number;
    error?: string;
    findingCount: number;
    findings: SecurityFinding[];
}

// SecurityTab renders the host's hardening posture as an interactive
// checklist. The data sources stack:
//
//   1. listAvailableChecks() — every checker the agent has registered,
//      whether or not it has ever run. Renders "not run yet" rows on
//      a fresh host so the operator can see what'll happen before
//      clicking.
//   2. getHostSecurityScan() — the latest persisted scan. Per-row
//      status (pass / findings / skipped / error) and findings come
//      from here.
//   3. rescanHost() — fires either the full registry or one check id.
//      Server-side merge logic keeps non-targeted findings intact, so
//      re-running ssh.config doesn't blank kernel/sysctl rows.
//
// While a mutation is in flight we mark only the *targeted* rows as
// "running" so the operator sees per-check spinners on partial reruns.
export default function SecurityTab({ projectID, hostID, active }: Props) {
    const { t } = useTranslation("security");
    const queryClient = useQueryClient();

    const checksQuery = useQuery({
        queryKey: qk.hostSecurityChecks(projectID, hostID),
        queryFn: () => listAvailableChecks(projectID, hostID),
        enabled: active,
        refetchOnWindowFocus: false,
    });

    const scanQuery = useQuery({
        queryKey: qk.hostSecurityScan(projectID, hostID),
        queryFn: () => getHostSecurityScan(projectID, hostID),
        enabled: active,
        refetchOnWindowFocus: false,
    });

    // Track which check ids are currently running so the per-row
    // spinner only lights up on the targeted rows. `null` means
    // "all checks running" (full re-scan).
    const [runningSet, setRunningSet] = useState<Set<string> | null>(null);

    const rescan = useMutation({
        mutationFn: (vars: { check_ids?: string[] }) =>
            rescanHost(projectID, hostID, vars),
        onMutate: (vars) => {
            setRunningSet(vars.check_ids ? new Set(vars.check_ids) : null);
        },
        onSettled: () => {
            setRunningSet(new Set());
        },
        onSuccess: (fresh) => {
            queryClient.setQueryData(qk.hostSecurityScan(projectID, hostID), fresh);
            queryClient.invalidateQueries({
                queryKey: qk.hostSecurityScans(projectID, hostID, 10),
            });
            queryClient.invalidateQueries({ queryKey: qk.hosts(projectID) });
            queryClient.invalidateQueries({
                queryKey: ["projectSecurityFindings", projectID],
            });
        },
    });

    const scan = scanQuery.data;
    const available = checksQuery.data;

    const rows = useMemo(
        () => buildRows(available, scan, runningSet),
        [available, scan, runningSet],
    );

    const counts: SeverityCounts =
        scan?.severity_counts ?? { critical: 0, high: 0, medium: 0, low: 0, info: 0 };

    const isAnyRunning = rescan.isPending;

    // First-load state: no checklist source available yet (agent
    // offline + never scanned). Show the placeholder empty state with
    // a single "Run scan" button — the agent doesn't have to be
    // online to display this; the click path will surface its own
    // error if the agent isn't reachable when triggered.
    const noChecklist = (rows.length === 0) && !checksQuery.isFetching && !scanQuery.isFetching;

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <Header
                scan={scan}
                counts={counts}
                isAnyRunning={isAnyRunning}
                onRescanAll={() => rescan.mutate({})}
            />

            {scanQuery.error && (
                <Alert kind="danger">{humanizeError(scanQuery.error)}</Alert>
            )}
            {rescan.error && (
                <Alert kind="warning">
                    {t("errors.scanFailed")} — {humanizeError(rescan.error)}
                </Alert>
            )}

            {noChecklist && (
                <EmptyState
                    icon={<Shield />}
                    title={t("neverScanned")}
                    description={t("neverScannedHint")}
                    action={
                        <Button
                            type="button"
                            size="sm"
                            onClick={() => rescan.mutate({})}
                            disabled={isAnyRunning}
                        >
                            {isAnyRunning ? t("rescan.running") : t("rescan.first")}
                        </Button>
                    }
                />
            )}

            {available && available.length > 0 && (
                <CoveragePanel checks={available} />
            )}

            {rows.length > 0 && (
                <Checklist
                    rows={rows}
                    onRerunOne={(id) => rescan.mutate({ check_ids: [id] })}
                    isAnyRunning={isAnyRunning}
                />
            )}
        </div>
    );
}

// CoveragePanel renders the full set of registered checks grouped
// by category, with each entry expandable to show its description
// and references. Collapsed by default to keep the per-host view
// from feeling cluttered; the "What do we check?" trigger expands
// it on demand. Honest scope-note at the top so operators
// immediately see this is not a CIS replacement.
function CoveragePanel({ checks }: { checks: AvailableCheck[] }) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);

    const grouped = useMemo(() => {
        const map = new Map<string, AvailableCheck[]>();
        for (const c of checks) {
            const arr = map.get(c.category) ?? [];
            arr.push(c);
            map.set(c.category, arr);
        }
        // Stable order: render categories alphabetically; within a
        // category render check ids alphabetically.
        return Array.from(map.entries())
            .sort((a, b) => a[0].localeCompare(b[0]))
            .map(([cat, items]) => [
                cat,
                items.slice().sort((a, b) => a.id.localeCompare(b.id)),
            ] as const);
    }, [checks]);

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
                    {t("coverage.heading")}
                </strong>
                <span style={{ fontSize: 12, color: palette.textSecondary }}>
                    {t("coverage.totalChecks", { count: checks.length })}
                </span>
                <span style={{ flex: 1 }} />
                <span style={{ fontSize: 12, color: palette.textMuted }}>
                    {open ? t("coverage.hide") : t("coverage.show")}
                </span>
            </button>
            {open && (
                <div
                    style={{
                        padding: `${space[3]}px ${space[3]}px`,
                        borderTop: `1px solid ${palette.border}`,
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                    }}
                >
                    <p style={{ margin: 0, fontSize: 12, color: palette.textSecondary }}>
                        {t("coverage.intro")}
                    </p>
                    <div
                        style={{
                            border: `1px solid ${palette.warning}`,
                            borderRadius: radius.sm,
                            padding: `${space[2]}px ${space[3]}px`,
                            background: "rgba(245, 166, 35, 0.08)",
                            fontSize: 12,
                            color: palette.textSecondary,
                            display: "flex",
                            flexDirection: "column",
                            gap: 4,
                        }}
                    >
                        <strong style={{ color: palette.warning }}>
                            {t("coverage.scopeNote")}
                        </strong>
                        <span>{t("coverage.scopeBody")}</span>
                    </div>
                    {grouped.map(([category, items]) => (
                        <CoverageGroup
                            key={category}
                            category={category}
                            items={items}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

function CoverageGroup({
    category,
    items,
}: {
    category: string;
    items: AvailableCheck[];
}) {
    const { t } = useTranslation("security");
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
            <span
                style={{
                    fontSize: 11,
                    fontWeight: 600,
                    textTransform: "uppercase",
                    color: palette.textMuted,
                    letterSpacing: 0.4,
                }}
            >
                {t(`category.${category}`, { defaultValue: category })}
            </span>
            <div style={{ display: "flex", flexDirection: "column", gap: space[1] }}>
                {items.map((c) => (
                    <CoverageRow key={c.id} c={c} />
                ))}
            </div>
        </div>
    );
}

function CoverageRow({ c }: { c: AvailableCheck }) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);
    const expandable = !!(c.description || (c.references && c.references.length > 0));

    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.sm,
                opacity: c.applicable ? 1 : 0.6,
                background: palette.main,
            }}
        >
            <button
                type="button"
                onClick={() => expandable && setOpen((v) => !v)}
                style={{
                    all: "unset",
                    cursor: expandable ? "pointer" : "default",
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: `${space[2]}px ${space[3]}px`,
                    width: "100%",
                    boxSizing: "border-box",
                }}
                aria-expanded={expandable ? open : undefined}
            >
                <span style={{ width: 16, display: "inline-flex" }}>
                    {expandable ? (
                        open ? (
                            <ChevronDown className="size-3" />
                        ) : (
                            <ChevronRight className="size-3" />
                        )
                    ) : null}
                </span>
                <Mono size={12}>{c.id}</Mono>
                <span style={{ flex: 1, fontSize: 12, color: palette.textSecondary }}>
                    {c.title || ""}
                </span>
                <span
                    style={{
                        fontSize: 11,
                        color: c.applicable ? palette.success : palette.textMuted,
                    }}
                >
                    {c.applicable ? t("coverage.applicable") : t("coverage.notApplicable")}
                </span>
            </button>
            {open && expandable && (
                <div
                    style={{
                        padding: `${space[2]}px ${space[3]}px ${space[3]}px ${space[6]}px`,
                        borderTop: `1px solid ${palette.border}`,
                        fontSize: 12,
                        color: palette.textSecondary,
                        display: "flex",
                        flexDirection: "column",
                        gap: space[2],
                    }}
                >
                    {c.description && (
                        <span style={{ whiteSpace: "pre-wrap" }}>{c.description}</span>
                    )}
                    {c.references && c.references.length > 0 && (
                        <div>
                            <strong style={{ fontSize: 11, color: palette.textMuted }}>
                                {t("coverage.referencesLabel")}:
                            </strong>{" "}
                            {c.references.map((r, i) => (
                                <span key={`${r}-${i}`}>
                                    {i > 0 ? ", " : ""}
                                    {linkifyReference(r)}
                                </span>
                            ))}
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}

// --- Checklist components ----------------------------------------------------

function Header({
    scan,
    counts,
    isAnyRunning,
    onRescanAll,
}: {
    scan: HostSecurityScan | null | undefined;
    counts: SeverityCounts;
    isAnyRunning: boolean;
    onRescanAll: () => void;
}) {
    const { t } = useTranslation("security");
    const scannedAt =
        scan?.started_at_unix ? new Date(scan.started_at_unix * 1000) : null;

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
            <Button
                type="button"
                size="sm"
                variant="default"
                onClick={onRescanAll}
                disabled={isAnyRunning}
            >
                {isAnyRunning ? (
                    <>
                        <Loader2 className="size-3.5 animate-spin" />
                        {t("rescan.running")}
                    </>
                ) : scan ? (
                    <>
                        <RefreshCw className="size-3.5" />
                        {t("checklist.rerunAll")}
                    </>
                ) : (
                    t("checklist.runAll")
                )}
            </Button>
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

function Checklist({
    rows,
    onRerunOne,
    isAnyRunning,
}: {
    rows: RowModel[];
    onRerunOne: (id: string) => void;
    isAnyRunning: boolean;
}) {
    const { t } = useTranslation("security");
    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                background: palette.surface,
                overflow: "hidden",
            }}
        >
            <div
                style={{
                    padding: `${space[2]}px ${space[3]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                    fontSize: 12,
                    color: palette.textSecondary,
                }}
            >
                <strong style={{ color: palette.textPrimary, fontSize: 13 }}>
                    {t("checklist.heading")}
                </strong>
                <span style={{ marginLeft: space[2] }}>{t("checklist.subheading")}</span>
            </div>
            <div>
                {rows.map((row) => (
                    <ChecklistRow
                        key={row.id}
                        row={row}
                        onRerun={() => onRerunOne(row.id)}
                        rerunDisabled={isAnyRunning || !row.applicable}
                    />
                ))}
            </div>
        </div>
    );
}

function ChecklistRow({
    row,
    onRerun,
    rerunDisabled,
}: {
    row: RowModel;
    onRerun: () => void;
    rerunDisabled: boolean;
}) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);
    const expandable = row.findings.length > 0 || (row.error && row.error !== "");

    return (
        <>
            <div
                onClick={() => expandable && setOpen((v) => !v)}
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[3],
                    padding: `${space[2]}px ${space[3]}px`,
                    borderBottom: `1px solid ${palette.border}`,
                    cursor: expandable ? "pointer" : "default",
                    opacity: row.applicable ? 1 : 0.55,
                }}
                aria-expanded={expandable ? open : undefined}
            >
                <span style={{ display: "inline-flex", alignItems: "center", width: 18 }}>
                    {expandable ? (
                        open ? (
                            <ChevronDown className="size-3.5" />
                        ) : (
                            <ChevronRight className="size-3.5" />
                        )
                    ) : null}
                </span>
                <StatusIcon status={row.status} />
                <span style={{ flex: 1, display: "flex", flexDirection: "column", gap: 2 }}>
                    <span style={{ display: "inline-flex", gap: space[2], alignItems: "center" }}>
                        <Mono size={13}>{row.id}</Mono>
                        <span
                            style={{
                                fontSize: 11,
                                color: palette.textMuted,
                                padding: `0 ${space[2]}px`,
                                borderRadius: radius.pill,
                                border: `1px solid ${palette.border}`,
                            }}
                        >
                            {row.category}
                        </span>
                        {!row.applicable && (
                            <span style={{ fontSize: 11, color: palette.textMuted }}>
                                {t("checklist.notApplicable")}
                            </span>
                        )}
                    </span>
                    <span style={{ fontSize: 12, color: palette.textSecondary }}>
                        <StatusLabel row={row} />
                        {row.elapsedMs != null && row.status !== "running" && (
                            <span style={{ marginLeft: space[2], color: palette.textMuted }}>
                                {row.elapsedMs}ms
                            </span>
                        )}
                    </span>
                </span>
                <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    onClick={(e) => {
                        e.stopPropagation();
                        onRerun();
                    }}
                    disabled={rerunDisabled}
                    title={t("checklist.rerunOne")}
                    aria-label={t("checklist.rerunOne")}
                >
                    {row.status === "running" ? (
                        <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                        <RefreshCw className="size-3.5" />
                    )}
                </Button>
            </div>
            {open && expandable && (
                <div
                    style={{
                        background: palette.surfaceHover,
                        borderBottom: `1px solid ${palette.border}`,
                        padding: `${space[3]}px ${space[5]}px`,
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                    }}
                >
                    {row.error && (
                        <DetailBlock label={t("details.description")} body={row.error} />
                    )}
                    {row.findings.map((f) => (
                        <FindingDetail key={f.id} f={f} />
                    ))}
                </div>
            )}
        </>
    );
}

function StatusIcon({ status }: { status: RowStatus }) {
    switch (status) {
        case "running":
            return <Loader2 className="size-4 animate-spin" style={{ color: palette.info }} />;
        case "pass":
            return <CheckCircle2 className="size-4" style={{ color: palette.success }} />;
        case "findings":
            return <AlertTriangle className="size-4" style={{ color: palette.warning }} />;
        case "skipped":
        case "not_applicable":
            return <MinusCircle className="size-4" style={{ color: palette.textMuted }} />;
        case "error":
            return <XCircle className="size-4" style={{ color: palette.danger }} />;
        default:
            return (
                <span
                    style={{
                        width: 16,
                        height: 16,
                        borderRadius: radius.pill,
                        border: `1px solid ${palette.border}`,
                    }}
                />
            );
    }
}

function StatusLabel({ row }: { row: RowModel }) {
    const { t } = useTranslation("security");
    switch (row.status) {
        case "running":
            return <>{t("checklist.statusRunning")}</>;
        case "pass":
            return <>{t("checklist.statusPass")}</>;
        case "findings":
            return <>{t("checklist.statusFindings", { count: row.findingCount })}</>;
        case "skipped":
            return <>{t("checklist.statusSkipped")}</>;
        case "error":
            return <>{t("checklist.statusError")}</>;
        case "not_applicable":
            return <>{t("checklist.statusSkipped")}</>;
        default:
            return <>{t("checklist.statusNotRun")}</>;
    }
}

function FindingDetail({ f }: { f: SecurityFinding }) {
    const { t } = useTranslation("security");
    const tone = severityTone(f.severity);
    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                padding: `${space[2]}px ${space[3]}px`,
                background: palette.surface,
                display: "flex",
                flexDirection: "column",
                gap: space[2],
                fontSize: 13,
            }}
        >
            <div style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
                <span
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
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
                <strong>{f.title}</strong>
            </div>
            <DetailBlock label={t("details.description")} body={f.description} />
            <DetailBlock label={t("details.evidence")} body={f.evidence} mono />
            <DetailBlock label={t("details.remediation")} body={f.remediation} />
            {f.references && f.references.length > 0 && (
                <div style={{ fontSize: 12 }}>
                    <strong style={{ color: palette.textSecondary }}>
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
        <div style={{ fontSize: 13 }}>
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

// buildRows fuses three data sources into one displayable list:
//   - available: ListSecurityChecks RPC (ground truth for "what
//     checks exist", regardless of whether they've run)
//   - scan.checks + scan.findings: per-check last-known status +
//     findings from the persisted scan
//   - runningSet: which check ids are currently being re-scanned
//
// Precedence: an entry in `available` always wins on identity (ids,
// applicability). When the agent is offline (`available` is null)
// we degrade to deriving the row set from the persisted scan's
// checks[]; if neither source exists, we render zero rows and the
// caller surfaces the never-scanned empty state.
function buildRows(
    available: AvailableCheck[] | null | undefined,
    scan: HostSecurityScan | null | undefined,
    runningSet: Set<string> | null,
): RowModel[] {
    const checkResults = new Map<string, SecurityCheckResult>();
    if (scan?.checks) {
        for (const c of scan.checks) checkResults.set(c.id, c);
    }
    const findingsByCheck = new Map<string, SecurityFinding[]>();
    if (scan?.findings) {
        for (const f of scan.findings) {
            const arr = findingsByCheck.get(f.check_id) ?? [];
            arr.push(f);
            findingsByCheck.set(f.check_id, arr);
        }
    }

    // Source of "all checks": prefer the live registry; fall back to
    // whatever the persisted scan saw.
    let source: { id: string; category: string; applicable: boolean }[];
    if (available && available.length > 0) {
        source = available;
    } else if (scan?.checks && scan.checks.length > 0) {
        source = scan.checks.map((c) => ({
            id: c.id,
            category: c.category,
            applicable: c.status !== "skipped",
        }));
    } else {
        return [];
    }

    return source
        .slice()
        .sort((a, b) => a.id.localeCompare(b.id))
        .map((meta): RowModel => {
            const result = checkResults.get(meta.id);
            const findings = findingsByCheck.get(meta.id) ?? [];
            // Per-row spinners light up only on PARTIAL reruns, where
            // the targeted check ids land in `runningSet`. Full
            // re-scans signal "in flight" only via the header button
            // — flipping every row's icon during a full scan made
            // the table flicker, since the scan typically finishes
            // before the next paint.
            const running = runningSet !== null && runningSet.has(meta.id);

            let status: RowStatus;
            if (running) {
                status = "running";
            } else if (!meta.applicable) {
                status = "not_applicable";
            } else if (!result) {
                status = "not_run";
            } else if (result.status === "error") {
                status = "error";
            } else if (result.status === "skipped") {
                status = "skipped";
            } else if (findings.length > 0) {
                status = "findings";
            } else {
                status = "pass";
            }

            return {
                id: meta.id,
                category: meta.category,
                applicable: meta.applicable,
                status,
                elapsedMs: result?.elapsed_ms,
                error: result?.error,
                findingCount: findings.length,
                findings,
            };
        });
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
