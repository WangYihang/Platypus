import * as React from "react";

import { palette, radius, space } from "../../layout/theme";

// Padding values must carry their `px` unit because the template literal
// turns `space[2]` (the number 8) into "8 12" — invalid CSS, silently
// dropped. Keep the explicit-unit pattern used elsewhere.

export const tableStyle: React.CSSProperties = {
    width: "100%",
    borderCollapse: "collapse",
    fontSize: 13,
};

export const thStyle: React.CSSProperties = {
    textAlign: "left",
    padding: `${space[2]}px ${space[3]}px`,
    fontSize: 11,
    fontWeight: 600,
    color: palette.textMuted,
    textTransform: "uppercase",
    letterSpacing: 0.4,
    borderBottom: `1px solid ${palette.border}`,
    whiteSpace: "nowrap",
};

export const thNumStyle: React.CSSProperties = {
    ...thStyle,
    textAlign: "right",
};

// Empty header label so the chevron itself is the affordance.
export const thChevronStyle: React.CSSProperties = {
    ...thStyle,
    width: 24,
    padding: `${space[2]}px ${space[1]}px`,
};

export const trStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
    cursor: "pointer",
};

export const trExpandedStyle: React.CSSProperties = {
    ...trStyle,
    background: palette.surfaceHover,
};

export const tdStyle: React.CSSProperties = {
    padding: `${space[2]}px ${space[3]}px`,
    color: palette.textPrimary,
    verticalAlign: "middle",
};

export const tdChevronStyle: React.CSSProperties = {
    ...tdStyle,
    padding: `${space[2]}px ${space[1]}px`,
    width: 24,
    color: palette.textMuted,
};

// Right-aligned + tabular-nums so digits line up across rows.
export const tdNumStyle: React.CSSProperties = {
    ...tdStyle,
    textAlign: "right",
    whiteSpace: "nowrap",
    fontVariantNumeric: "tabular-nums",
};

export const tdPathStyle: React.CSSProperties = {
    ...tdStyle,
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    color: palette.textSecondary,
    maxWidth: 360,
    overflow: "hidden",
};

export const pathInnerStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    gap: space[2],
    minWidth: 0,
};

export const pathTextStyle: React.CSSProperties = {
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
};

// Stacked "X / Y" with a smaller muted sub-line beneath.
export const sizeMainStyle: React.CSSProperties = {
    fontVariantNumeric: "tabular-nums",
};

export const sizeSubStyle: React.CSSProperties = {
    fontSize: 11,
    color: palette.textMuted,
    fontVariantNumeric: "tabular-nums",
    marginTop: 1,
};

// Single full-width red strip immediately under the row so the operator
// sees what failed without expanding.
export const inlineErrorRowStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
};

export const inlineErrorCellStyle: React.CSSProperties = {
    padding: `${space[1]}px ${space[3]}px ${space[2]}px ${space[3]}px`,
    color: palette.danger,
    fontSize: 12,
    background: palette.surface,
};

// Expanded detail row: full-width cell with a two-column key-value grid.
export const detailRowStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
    background: palette.surfaceHover,
};

export const detailCellStyle: React.CSSProperties = {
    padding: `${space[3]}px ${space[5]}px`,
};

export const detailGridStyle: React.CSSProperties = {
    display: "grid",
    gridTemplateColumns: "1fr 1fr",
    gap: `${space[2]}px ${space[5]}px`,
    margin: 0,
    fontSize: 12,
};

export const detailKeyStyle: React.CSSProperties = {
    color: palette.textMuted,
    minWidth: 100,
    whiteSpace: "nowrap",
    fontWeight: 500,
};

export const detailValueStyle: React.CSSProperties = {
    color: palette.textPrimary,
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    fontVariantNumeric: "tabular-nums",
};

// Track + label side-by-side. Fixed track width + min-width keeps the
// column from collapsing when the table fights for space.
export const progressWrapperStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    gap: space[2],
    minWidth: 180,
};

export const progressTrackStyle: React.CSSProperties = {
    position: "relative",
    width: 140,
    height: 6,
    background: palette.border,
    borderRadius: radius.pill,
    overflow: "hidden",
    flexShrink: 0,
};

export const progressLabelStyle: React.CSSProperties = {
    fontSize: 11,
    color: palette.textMuted,
    fontVariantNumeric: "tabular-nums",
    minWidth: 32,
    textAlign: "right",
};
