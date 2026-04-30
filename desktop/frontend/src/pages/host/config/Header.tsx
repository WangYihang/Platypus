import { Loader2, RefreshCw, KeyRound } from "lucide-react";

import { Button } from "@/components/ui/button";

import { palette, space } from "../../../layout/theme";
import type { HostConfigAudit } from "../../../lib/api";
import { fromNow } from "../../../lib/time";

import { RISKS, riskTone } from "./riskTone";
import RiskChip from "./RiskChip";

interface Props {
    audit: HostConfigAudit | null | undefined;
    isAnyRunning: boolean;
    onReauditAll: () => void;
}

// Header is the persistent top strip of the Config tab. Three jobs:
//   1. Identify the surface (icon + "Configuration audit" label) so a
//      first-time visitor knows they're not in the security tab.
//   2. Show the most recent audit's wall-clock + risk histogram so
//      the operator gets the headline number at a glance.
//   3. Provide the single "Re-audit" affordance that triggers a fresh
//      full audit. Disabled while any audit (full or partial) is
//      running — partial-row reruns are wired via the auditor list.
export function Header({ audit, isAnyRunning, onReauditAll }: Props) {
    const counts = audit?.risk_counts ?? { high: 0, medium: 0, low: 0, info: 0 };
    const total = counts.high + counts.medium + counts.low + counts.info;

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[3],
                paddingBottom: space[2],
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <KeyRound className="size-4" style={{ color: palette.warning }} />
            <strong style={{ fontSize: 14, color: palette.textPrimary }}>
                Configuration audit
            </strong>

            {audit ? (
                <span style={{ fontSize: 12, color: palette.textMuted }}>
                    last run {fromNow(new Date(audit.started_at_unix * 1000))}
                </span>
            ) : (
                <span style={{ fontSize: 12, color: palette.textMuted }}>
                    not audited yet
                </span>
            )}

            {audit && total > 0 && (
                <div style={{ display: "inline-flex", gap: 6 }}>
                    {RISKS.map((r) => {
                        const n =
                            r === "high"
                                ? counts.high
                                : r === "medium"
                                ? counts.medium
                                : r === "low"
                                ? counts.low
                                : counts.info;
                        if (n === 0) return null;
                        return <RiskChip key={r} risk={r} count={n} />;
                    })}
                </div>
            )}
            {audit && total === 0 && (
                <span
                    style={{
                        fontSize: 11,
                        color: riskTone("info").fg,
                        background: "rgba(62, 207, 142, 0.1)",
                        padding: "2px 8px",
                        borderRadius: 999,
                        fontWeight: 600,
                        textTransform: "uppercase",
                        letterSpacing: 0.4,
                    }}
                >
                    Clean
                </span>
            )}

            <span style={{ flex: 1 }} />

            <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onReauditAll}
                disabled={isAnyRunning}
            >
                {isAnyRunning ? (
                    <>
                        <Loader2 className="size-3.5 animate-spin" /> Auditing…
                    </>
                ) : (
                    <>
                        <RefreshCw className="size-3.5" />{" "}
                        {audit ? "Re-audit" : "Run audit"}
                    </>
                )}
            </Button>
        </div>
    );
}

export default Header;
