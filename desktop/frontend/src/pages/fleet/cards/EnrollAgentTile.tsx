import { Plus } from "lucide-react";
import { Link } from "react-router-dom";

import { palette, radius, space } from "../../../layout/theme";

// EnrollAgentTile is the "+" entry card that lives as the first
// tile in the Fleet card grid. Visually distinct from a host card —
// dashed border, centered + glyph, no data rows — so the eye
// doesn't read it as a real host. Clicking sets `?enroll=1` on the
// current location, which the EnrollAgentWizard picks up and opens.
//
// Lives as a tile (not a separate header button) because the user
// wants the entry point inside the same surface they're scanning to
// see the fleet — adding the first machine should feel like
// "filling in the next square".
export default function EnrollAgentTile() {
    return (
        <Link
            to="?enroll=1"
            data-testid="fleet-enroll-tile"
            style={{
                textDecoration: "none",
                background: "transparent",
                border: `1px dashed ${palette.border}`,
                borderRadius: radius.md,
                padding: `${space[4]}px ${space[4]}px ${space[3]}px`,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                gap: space[2],
                color: palette.textSecondary,
                cursor: "pointer",
                fontFamily: "var(--font-geist-mono)",
                minHeight: 180,
                transition: "border-color 120ms ease, color 120ms ease",
            }}
            onMouseEnter={(e) => {
                e.currentTarget.style.borderColor = palette.accent;
                e.currentTarget.style.color = palette.textPrimary;
            }}
            onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = palette.border;
                e.currentTarget.style.color = palette.textSecondary;
            }}
        >
            <span
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    width: 32,
                    height: 32,
                    borderRadius: 999,
                    border: `1px solid currentColor`,
                }}
            >
                <Plus className="size-4" />
            </span>
            <span style={{ fontWeight: 600, fontSize: 14 }}>Enroll agent</span>
            <span
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    textAlign: "center",
                    lineHeight: 1.4,
                }}
            >
                Multi-step wizard — pick OS, arch, get a one-shot install
                command.
            </span>
        </Link>
    );
}
