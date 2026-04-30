import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageShell from "../components/PageShell";
import RefreshButton from "../components/RefreshButton";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, radius, space } from "../layout/theme";
import {
    Host,
    SecurityFinding,
    Severity,
    listHosts,
    listProjectFindings,
} from "../lib/api";
import { humanizeError } from "../lib/humanizeError";
import { qk } from "../lib/queryKeys";
import { fromNow } from "../lib/time";

import { severityTone } from "./fleet/cards/SecurityBadge";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const SEVERITIES: Severity[] = ["critical", "high", "medium", "low", "info"];
const CATEGORIES = ["kernel", "sysctl", "ssh", "filesystem"];
const PAGE_SIZE = 50;

// SecurityPage is the project-level cross-host findings table. The
// per-host detail tab lives at /hosts/:id/security; this page
// answers "show me every Critical finding in this project" with a
// single SQL query (server-side LatestFindings). Filter state is
// reflected in URL search params so reloads / back-button keep
// place.
export default function SecurityPage() {
    const { t } = useTranslation("security");
    const project = useCurrentProject();
    const projectID = project.id;

    const [params, setParams] = useSearchParams();

    const severity = useMemo(
        () => splitParam(params.get("severity")) as Severity[],
        [params],
    );
    const category = useMemo(() => splitParam(params.get("category")), [params]);
    const hostID = params.get("host_id") ?? "";
    const q = params.get("q") ?? "";
    const page = Math.max(1, Number(params.get("page") ?? "1") | 0);

    const opts = {
        severity: severity.length ? severity : undefined,
        category: category.length ? category : undefined,
        host_id: hostID || undefined,
        q: q || undefined,
        page,
        page_size: PAGE_SIZE,
    };

    const findingsQuery = useQuery({
        queryKey: qk.projectSecurityFindings(projectID, opts),
        queryFn: () => listProjectFindings(projectID, opts),
    });

    // Hosts list reused for the host-filter dropdown. React Query
    // shares the cache with FleetPage so navigating from Fleet →
    // Security skips the round trip.
    const hostsQuery = useQuery({
        queryKey: qk.hosts(projectID),
        queryFn: () => listHosts(projectID),
    });

    const total = findingsQuery.data?.total ?? 0;
    const findings = findingsQuery.data?.findings ?? [];
    const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
    const hasFilters =
        severity.length > 0 || category.length > 0 || hostID !== "" || q !== "";

    function patchParams(next: Record<string, string | string[] | null>) {
        const p = new URLSearchParams(params);
        for (const [k, v] of Object.entries(next)) {
            if (v == null || (Array.isArray(v) && v.length === 0) || v === "") {
                p.delete(k);
            } else if (Array.isArray(v)) {
                p.set(k, v.join(","));
            } else {
                p.set(k, v);
            }
        }
        // Any filter change resets pagination.
        if (Object.keys(next).some((k) => k !== "page")) {
            p.delete("page");
        }
        setParams(p, { replace: true });
    }

    return (
        <PageShell
            title={t("page.title")}
            subtitle={t("page.subtitle")}
            actions={
                <RefreshButton
                    loading={findingsQuery.isFetching}
                    onClick={() => void findingsQuery.refetch()}
                />
            }
        >
            <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                <FilterBar
                    severity={severity}
                    category={category}
                    hostID={hostID}
                    q={q}
                    hosts={hostsQuery.data ?? []}
                    onSeverity={(v) => patchParams({ severity: v })}
                    onCategory={(v) => patchParams({ category: v })}
                    onHostID={(v) => patchParams({ host_id: v })}
                    onQuery={(v) => patchParams({ q: v })}
                    onClear={() =>
                        patchParams({
                            severity: null,
                            category: null,
                            host_id: null,
                            q: null,
                            page: null,
                        })
                    }
                    hasFilters={hasFilters}
                />

                {findingsQuery.error && (
                    <div
                        style={{
                            padding: `${space[2]}px ${space[3]}px`,
                            border: `1px solid ${palette.danger}`,
                            borderRadius: radius.md,
                            color: palette.danger,
                            fontSize: 12,
                        }}
                    >
                        {humanizeError(findingsQuery.error)}
                    </div>
                )}

                {!findingsQuery.isFetching && findings.length === 0 && (
                    <EmptyState
                        title={t("empty.title")}
                        description={
                            hasFilters
                                ? t("filter.clear")
                                : t("empty.subtitle")
                        }
                    />
                )}

                {findings.length > 0 && (
                    <FindingsTable
                        findings={findings}
                        hosts={hostsQuery.data ?? []}
                        projectSlug={project.slug}
                    />
                )}

                <PaginationFooter
                    total={total}
                    page={page}
                    totalPages={totalPages}
                    onPage={(p) => patchParams({ page: String(p) })}
                />
            </div>
        </PageShell>
    );
}

function FilterBar({
    severity,
    category,
    hostID,
    q,
    hosts,
    onSeverity,
    onCategory,
    onHostID,
    onQuery,
    onClear,
    hasFilters,
}: {
    severity: Severity[];
    category: string[];
    hostID: string;
    q: string;
    hosts: Host[];
    onSeverity: (v: string[]) => void;
    onCategory: (v: string[]) => void;
    onHostID: (v: string) => void;
    onQuery: (v: string) => void;
    onClear: () => void;
    hasFilters: boolean;
}) {
    const { t } = useTranslation("security");
    return (
        <div
            style={{
                display: "flex",
                gap: space[3],
                flexWrap: "wrap",
                alignItems: "center",
                fontSize: 12,
                color: palette.textSecondary,
            }}
        >
            <FilterLabel>{t("filter.severity")}:</FilterLabel>
            <ToggleGroup
                type="multiple"
                value={severity}
                onValueChange={(v) => onSeverity(v as Severity[])}
                size="sm"
            >
                {SEVERITIES.map((s) => (
                    <ToggleGroupItem key={s} value={s}>
                        {t(`severity.${s}`)}
                    </ToggleGroupItem>
                ))}
            </ToggleGroup>

            <FilterLabel>{t("filter.category")}:</FilterLabel>
            <ToggleGroup
                type="multiple"
                value={category}
                onValueChange={onCategory}
                size="sm"
            >
                {CATEGORIES.map((c) => (
                    <ToggleGroupItem key={c} value={c}>
                        {t(`category.${c}`, { defaultValue: c })}
                    </ToggleGroupItem>
                ))}
            </ToggleGroup>

            <FilterLabel>{t("filter.host")}:</FilterLabel>
            <Select value={hostID || "_all"} onValueChange={(v) => onHostID(v === "_all" ? "" : v)}>
                <SelectTrigger style={{ minWidth: 200 }} size="sm">
                    <SelectValue placeholder={t("filter.allHosts")} />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="_all">{t("filter.allHosts")}</SelectItem>
                    {hosts.map((h) => (
                        <SelectItem key={h.id} value={h.id}>
                            {hostLabel(h)}
                        </SelectItem>
                    ))}
                </SelectContent>
            </Select>

            <Input
                value={q}
                onChange={(e) => onQuery(e.target.value)}
                placeholder={t("filter.search")}
                style={{ flex: "1 1 240px", minWidth: 200, maxWidth: 320 }}
            />

            {hasFilters && (
                <Button type="button" size="sm" variant="ghost" onClick={onClear}>
                    {t("filter.clear")}
                </Button>
            )}
        </div>
    );
}

function FilterLabel({ children }: { children: React.ReactNode }) {
    return (
        <span style={{ color: palette.textMuted, fontSize: 12 }}>{children}</span>
    );
}

function FindingsTable({
    findings,
    hosts,
    projectSlug,
}: {
    findings: SecurityFinding[];
    hosts: Host[];
    projectSlug: string;
}) {
    const { t } = useTranslation("security");
    const hostLookup = useMemo(() => {
        const m = new Map<string, Host>();
        for (const h of hosts) m.set(h.id, h);
        return m;
    }, [hosts]);

    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead className="w-[110px]">{t("table.severity")}</TableHead>
                    <TableHead>{t("table.title")}</TableHead>
                    <TableHead className="w-[200px]">{t("table.host")}</TableHead>
                    <TableHead className="w-[110px]">{t("table.category")}</TableHead>
                    <TableHead className="w-[140px]">{t("table.check")}</TableHead>
                </TableRow>
            </TableHeader>
            <TableBody>
                {findings.map((f) => (
                    <FindingRow
                        key={f.id}
                        f={f}
                        host={f.host_id ? hostLookup.get(f.host_id) : undefined}
                        projectSlug={projectSlug}
                    />
                ))}
            </TableBody>
        </Table>
    );
}

function FindingRow({
    f,
    host,
    projectSlug,
}: {
    f: SecurityFinding;
    host: Host | undefined;
    projectSlug: string;
}) {
    const { t } = useTranslation("security");
    const [open, setOpen] = useState(false);
    const tone = severityTone(f.severity);
    const hostHref = f.host_id
        ? `/projects/${projectSlug}/hosts/${f.host_id}/security`
        : null;

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
                    <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
                        {open ? (
                            <ChevronDown className="size-3.5" />
                        ) : (
                            <ChevronRight className="size-3.5" />
                        )}
                        <span>{f.title}</span>
                    </span>
                </TableCell>
                <TableCell>
                    {hostHref ? (
                        <Link
                            to={hostHref}
                            onClick={(e) => e.stopPropagation()}
                            style={{ color: palette.info }}
                        >
                            {host ? hostLabel(host) : f.host_id}
                        </Link>
                    ) : (
                        <span style={{ color: palette.textMuted }}>—</span>
                    )}
                </TableCell>
                <TableCell>
                    <span style={{ fontSize: 12, color: palette.textSecondary }}>
                        {t(`category.${f.category}`, { defaultValue: f.category })}
                    </span>
                </TableCell>
                <TableCell>
                    <Mono size={11}>{f.check_id || f.finding_id}</Mono>
                </TableCell>
            </TableRow>
            {open && (
                <TableRow>
                    <TableCell colSpan={5} style={{ background: palette.surfaceHover }}>
                        <div
                            style={{
                                display: "flex",
                                flexDirection: "column",
                                gap: space[2],
                                padding: `${space[2]}px 0`,
                                fontSize: 13,
                            }}
                        >
                            <Block label={t("details.description")} body={f.description} />
                            <Block label={t("details.evidence")} body={f.evidence} mono />
                            <Block label={t("details.remediation")} body={f.remediation} />
                            {f.scanned_at_unix ? (
                                <span style={{ fontSize: 12, color: palette.textMuted }}>
                                    {t("lastScanned", {
                                        when: fromNow(new Date(f.scanned_at_unix * 1000)),
                                    })}
                                </span>
                            ) : null}
                        </div>
                    </TableCell>
                </TableRow>
            )}
        </>
    );
}

function Block({
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
            <strong style={{ color: palette.textSecondary, fontSize: 12 }}>{label}:</strong>{" "}
            {mono ? (
                <Mono size={12}>{body}</Mono>
            ) : (
                <span style={{ whiteSpace: "pre-wrap" }}>{body}</span>
            )}
        </div>
    );
}

function PaginationFooter({
    total,
    page,
    totalPages,
    onPage,
}: {
    total: number;
    page: number;
    totalPages: number;
    onPage: (p: number) => void;
}) {
    const { t } = useTranslation("security");
    if (total === 0) return null;
    return (
        <div
            style={{
                display: "flex",
                gap: space[3],
                alignItems: "center",
                justifyContent: "space-between",
                fontSize: 12,
                color: palette.textSecondary,
            }}
        >
            <span>{t("page.totalCount", { count: total })}</span>
            <span style={{ display: "inline-flex", gap: space[2], alignItems: "center" }}>
                <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => onPage(page - 1)}
                    disabled={page <= 1}
                >
                    {t("pagination.prev")}
                </Button>
                <span>
                    {t("pagination.pageOfTotal", { page, total: totalPages })}
                </span>
                <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => onPage(page + 1)}
                    disabled={page >= totalPages}
                >
                    {t("pagination.next")}
                </Button>
            </span>
        </div>
    );
}

function splitParam(s: string | null): string[] {
    if (!s) return [];
    return s
        .split(",")
        .map((x) => x.trim())
        .filter(Boolean);
}

function hostLabel(h: Host): string {
    return h.primary_alias || h.hostname || h.machine_id?.slice(0, 12) || h.id;
}
