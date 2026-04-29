import { Fragment, ReactNode } from "react";

import { palette, space } from "../layout/theme";

import { Checkbox } from "@/components/ui/checkbox";

// FacetSidebar is the left rail used by list pages with multi-axis
// filtering (Audit being the canonical consumer). Each facet is a
// titled checkbox group: title (USER / EVENT / etc.) on top, an
// option row per value with the option label + a numeric count on
// the right. Selected options are checked; clicking toggles the
// option's membership in the selection set.
//
// The component is presentational — it receives the facets +
// selection state from the caller and emits onToggle. The owning
// page is responsible for computing facet counts from the current
// result set and persisting selection in URL/state/preferences.
//
// Facets render in source order so callers control axis ordering.
// Empty facets (zero options) are filtered out automatically so
// the sidebar doesn't grow blank "User" / "Event" headings on a
// fresh page.

export interface FacetOption {
    value: string;
    label?: ReactNode; // defaults to `value`
    count: number;
    selected: boolean;
}

export interface Facet {
    key: string;
    title: string;
    options: FacetOption[];
}

interface Props {
    facets: Facet[];
    onToggle: (facetKey: string, value: string) => void;
    width?: number;
}

export default function FacetSidebar({ facets, onToggle, width = 240 }: Props) {
    const visible = facets.filter((f) => f.options.length > 0);
    if (visible.length === 0) return null;
    return (
        <aside
            data-testid="facet-sidebar"
            style={{
                width,
                minWidth: width,
                flexShrink: 0,
                borderRight: `1px solid ${palette.border}`,
                padding: `${space[3]}px ${space[3]}px ${space[5]}px`,
                overflow: "auto",
                display: "flex",
                flexDirection: "column",
                gap: space[4],
            }}
        >
            {visible.map((f) => (
                <Fragment key={f.key}>
                    <div>
                        <div
                            data-testid={`facet-title-${f.key}`}
                            style={{
                                color: palette.textMuted,
                                fontSize: 10,
                                fontWeight: 600,
                                letterSpacing: 0.6,
                                textTransform: "uppercase",
                                marginBottom: space[2],
                            }}
                        >
                            {f.title}
                        </div>
                        <div
                            style={{
                                display: "flex",
                                flexDirection: "column",
                                gap: 2,
                            }}
                        >
                            {f.options.map((opt) => (
                                <label
                                    key={opt.value}
                                    style={{
                                        display: "flex",
                                        alignItems: "center",
                                        gap: space[2],
                                        padding: "4px 6px",
                                        borderRadius: 4,
                                        cursor: "pointer",
                                        color: palette.textPrimary,
                                        fontSize: 12,
                                    }}
                                >
                                    <Checkbox
                                        checked={opt.selected}
                                        onCheckedChange={() => onToggle(f.key, opt.value)}
                                        aria-label={`${f.title}: ${opt.value}`}
                                    />
                                    <span
                                        style={{
                                            flex: 1,
                                            minWidth: 0,
                                            overflow: "hidden",
                                            textOverflow: "ellipsis",
                                            whiteSpace: "nowrap",
                                        }}
                                    >
                                        {opt.label ?? opt.value}
                                    </span>
                                    <span
                                        style={{
                                            color: palette.textMuted,
                                            fontSize: 11,
                                        }}
                                    >
                                        {opt.count}
                                    </span>
                                </label>
                            ))}
                        </div>
                    </div>
                </Fragment>
            ))}
        </aside>
    );
}
