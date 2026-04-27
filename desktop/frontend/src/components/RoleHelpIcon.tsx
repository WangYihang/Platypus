import { HelpCircle } from "lucide-react";

import { palette } from "../layout/theme";
import { formatRoleSummary } from "../lib/roles";

// RoleHelpIcon is a small help affordance to drop next to any column
// header that displays a role label. It surfaces the per-role
// permission summary from lib/roles via a native title= attribute so
// the explanation is reachable without depending on a tooltip
// portal — fast, no extra Radix wiring, and screen-reader-friendly
// via the matching aria-label.
//
// Why a button instead of a span: native focus + a focusable target
// keeps keyboard users on equal footing with mouse users for hover
// tooltips, and announces the help text on focus.
export default function RoleHelpIcon() {
    const summary = formatRoleSummary();
    return (
        <button
            type="button"
            aria-label={`Role descriptions: ${summary}`}
            title={summary}
            // Preventing default so clicks don't trigger a nearby
            // form submit if this lives inside one.
            onClick={(e) => e.preventDefault()}
            style={{
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                width: 16,
                height: 16,
                padding: 0,
                marginLeft: 4,
                background: "none",
                border: "none",
                color: palette.textMuted,
                cursor: "help",
                verticalAlign: "middle",
            }}
        >
            <HelpCircle className="size-3" />
        </button>
    );
}
