// Single source of truth for risk → colour/label mapping. Lifted into
// its own module so every component (Header chips, LeakRow badge,
// auditor checklist) renders the same colours; changing the
// vocabulary or the palette here updates the whole tab.

import { palette } from "../../../layout/theme";
import type { Risk } from "../../../lib/api";

export const RISKS: Risk[] = ["high", "medium", "low", "info"];

interface RiskTone {
    label: string;
    fg: string;
    bg: string;
    /** order index, lower = higher risk; used by sort comparators. */
    rank: number;
}

const TONE: Record<Risk, RiskTone> = {
    high: {
        label: "High",
        fg: palette.danger,
        bg: "rgba(238, 0, 0, 0.12)",
        rank: 0,
    },
    medium: {
        label: "Medium",
        fg: palette.warning,
        bg: "rgba(245, 166, 35, 0.14)",
        rank: 1,
    },
    low: {
        label: "Low",
        fg: palette.info,
        bg: "rgba(0, 112, 243, 0.14)",
        rank: 2,
    },
    info: {
        label: "Info",
        fg: palette.textSecondary,
        bg: "rgba(255, 255, 255, 0.06)",
        rank: 3,
    },
};

export function riskTone(risk: Risk): RiskTone {
    return TONE[risk] ?? TONE.info;
}

export function compareRisk(a: Risk, b: Risk): number {
    return riskTone(a).rank - riskTone(b).rank;
}
