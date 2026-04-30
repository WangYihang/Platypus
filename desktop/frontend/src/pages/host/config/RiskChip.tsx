import type { Risk } from "../../../lib/api";
import { riskTone } from "./riskTone";

interface Props {
    risk: Risk;
    /** Optional count to render after the label (e.g. "High 3"). */
    count?: number;
    /** Compact variant: smaller padding for inline use inside dense rows. */
    dense?: boolean;
}

// RiskChip renders a single risk pill. Used by Header for the count
// histogram and by LeakRow for the row's own risk tag. Centralised so
// the colour vocabulary stays consistent — adding a new risk level is
// a one-file change in riskTone.ts.
export function RiskChip({ risk, count, dense }: Props) {
    const tone = riskTone(risk);
    const padding = dense ? "1px 6px" : "2px 8px";
    const fontSize = dense ? 10 : 11;
    return (
        <span
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                padding,
                fontSize,
                fontWeight: 600,
                color: tone.fg,
                background: tone.bg,
                borderRadius: 999,
                lineHeight: 1.4,
                textTransform: "uppercase",
                letterSpacing: 0.4,
            }}
        >
            {tone.label}
            {typeof count === "number" && (
                <span style={{ fontWeight: 500, opacity: 0.85 }}>
                    {count}
                </span>
            )}
        </span>
    );
}

export default RiskChip;
