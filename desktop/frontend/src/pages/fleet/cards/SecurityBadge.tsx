import { ShieldAlert, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";

import { palette, radius, space } from "../../../layout/theme";
import type { SeverityCounts } from "../../../lib/api";
import { fromNow } from "../../../lib/time";

interface Props {
    counts?: SeverityCounts | null;
    scannedAtUnix?: number;
    // Compact mode strips the count text and renders just the icon
    // pill — used on dense surfaces (HostCard) where horizontal space
    // is tight. The full card view passes false for the inline tab
    // header.
    compact?: boolean;
}

// SecurityBadge renders one of three states from the host's most-recent
// scan summary:
//
//   counts == null/undefined          → host never scanned, no badge.
//   counts present, every severity 0  → tiny green check ("clean").
//   counts present, any severity > 0  → coloured pill keyed off the
//                                       worst severity present.
//
// The "never scanned" state deliberately renders nothing rather than
// a grey placeholder — it keeps unscanned hosts visually identical to
// pre-feature builds, so adopting persistence doesn't make every
// existing card sprout a "no data" pill on first load.
export default function SecurityBadge({ counts, scannedAtUnix, compact = false }: Props) {
    const { t } = useTranslation("security");

    if (!counts) return null;

    const worst = pickWorst(counts);
    const scannedAt =
        scannedAtUnix && scannedAtUnix > 0
            ? new Date(scannedAtUnix * 1000)
            : null;

    if (!worst) {
        // Scanned, clean. Keep the pill small + green so it reads as a
        // pass at a glance.
        const tip = [
            t("indicator.clean"),
            scannedAt ? t("lastScanned", { when: fromNow(scannedAt) }) : null,
        ]
            .filter(Boolean)
            .join(" — ");
        return (
            <span
                title={tip}
                aria-label={tip}
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    color: palette.success,
                    flexShrink: 0,
                }}
            >
                <ShieldCheck className="size-3.5" />
            </span>
        );
    }

    const { tone, label, count } = worst;
    const tip = [
        t("indicator.tooltip", {
            critical: counts.critical,
            high: counts.high,
            medium: counts.medium,
        }),
        scannedAt ? t("lastScanned", { when: fromNow(scannedAt) }) : null,
    ]
        .filter(Boolean)
        .join(" — ");

    return (
        <span
            title={tip}
            aria-label={tip}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: space[1],
                padding: `2px ${compact ? 4 : space[2]}px`,
                borderRadius: radius.pill,
                background: tone.background,
                color: tone.foreground,
                border: `1px solid ${tone.border}`,
                fontSize: 11,
                fontWeight: 600,
                lineHeight: 1.2,
                flexShrink: 0,
            }}
        >
            <ShieldAlert className="size-3" />
            {compact ? count : `${label} ${count}`}
        </span>
    );
}

// severityTone returns the swatch for a worst-severity pill. Reuses
// the global palette tokens rather than introducing a parallel
// per-severity palette so every consumer (this badge, the per-host
// table rows, the project-page rows) renders the same hues.
export function severityTone(severity: "critical" | "high" | "medium" | "low" | "info") {
    switch (severity) {
        case "critical":
            return {
                background: "rgba(238, 0, 0, 0.18)",
                foreground: "#ff8b8b",
                border: "rgba(238, 0, 0, 0.42)",
            };
        case "high":
            return {
                background: "rgba(245, 166, 35, 0.18)",
                foreground: "#f5a623",
                border: "rgba(245, 166, 35, 0.42)",
            };
        case "medium":
            return {
                background: "rgba(245, 200, 35, 0.16)",
                foreground: "#e3c54a",
                border: "rgba(245, 200, 35, 0.36)",
            };
        case "low":
            return {
                background: "rgba(161, 161, 161, 0.18)",
                foreground: "#cfcfcf",
                border: "rgba(161, 161, 161, 0.36)",
            };
        case "info":
            return {
                background: "rgba(0, 112, 243, 0.18)",
                foreground: "#7ab7ff",
                border: "rgba(0, 112, 243, 0.36)",
            };
    }
}

function pickWorst(counts: SeverityCounts) {
    if (counts.critical > 0) {
        return {
            tone: severityTone("critical"),
            label: "Crit",
            count: counts.critical,
        };
    }
    if (counts.high > 0) {
        return { tone: severityTone("high"), label: "High", count: counts.high };
    }
    if (counts.medium > 0) {
        return {
            tone: severityTone("medium"),
            label: "Med",
            count: counts.medium,
        };
    }
    if (counts.low > 0) {
        return { tone: severityTone("low"), label: "Low", count: counts.low };
    }
    return null;
}
