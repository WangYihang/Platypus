import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";

import Mono from "../../../components/Mono";
import { palette, radius, space } from "../../../layout/theme";
import type { ConfigLeak } from "../../../lib/api";

import MatchCell from "./MatchCell";
import RiskChip from "./RiskChip";

interface Props {
    leak: ConfigLeak;
}

// LeakRow is a single expandable row in the leaks list. The collapsed
// row is dense by design — risk chip, pattern, location, redacted
// match — so an operator can scan a long list without scrolling each
// row's full description. Expanded reveals description / remediation /
// references.
export function LeakRow({ leak }: Props) {
    const [open, setOpen] = useState(false);
    return (
        <div
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: radius.sm,
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
                <RiskChip risk={leak.risk} dense />
                <span title={leak.pattern}>
                    <Mono
                        style={{
                            fontSize: 12,
                            color: palette.textPrimary,
                            whiteSpace: "nowrap",
                        }}
                    >
                        {leak.pattern}
                    </Mono>
                </span>
                <span
                    style={{
                        fontSize: 12,
                        color: palette.textSecondary,
                        flex: 1,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                    }}
                    title={leak.title}
                >
                    {leak.title}
                </span>
                <span title={leak.location}>
                    <Mono
                        style={{
                            fontSize: 11,
                            color: palette.textMuted,
                            whiteSpace: "nowrap",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            maxWidth: 280,
                            display: "inline-block",
                        }}
                    >
                        {leak.location}
                    </Mono>
                </span>
                <MatchCell value={leak.match} />
            </button>
            {open && <Details leak={leak} />}
        </div>
    );
}

function Details({ leak }: { leak: ConfigLeak }) {
    return (
        <div
            style={{
                padding: space[3],
                borderTop: `1px solid ${palette.border}`,
                display: "flex",
                flexDirection: "column",
                gap: space[2],
                fontSize: 12,
                color: palette.textSecondary,
                lineHeight: 1.55,
            }}
        >
            {leak.description && (
                <Section label="Description">{leak.description}</Section>
            )}
            {leak.remediation && (
                <Section label="Remediation">{leak.remediation}</Section>
            )}
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "auto 1fr",
                    gap: `4px ${space[3]}px`,
                    fontSize: 11,
                    color: palette.textMuted,
                }}
            >
                <span>Auditor</span>
                <Mono>{leak.auditor_id}</Mono>
                <span>Category</span>
                <Mono>{leak.category}</Mono>
                <span>Leak ID</span>
                <Mono>{leak.leak_id}</Mono>
            </div>
            {leak.references && leak.references.length > 0 && (
                <Section label="References">
                    <ul style={{ margin: 0, paddingLeft: space[4] }}>
                        {leak.references.map((r) => (
                            <li key={r}>{r}</li>
                        ))}
                    </ul>
                </Section>
            )}
        </div>
    );
}

function Section({
    label,
    children,
}: {
    label: string;
    children: React.ReactNode;
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
                {label}
            </div>
            <div>{children}</div>
        </div>
    );
}

export default LeakRow;
