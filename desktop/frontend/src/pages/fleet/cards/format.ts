import { Host } from "../../../lib/api";

// formatMem renders a byte count as a human-readable size with
// one or zero fractional digits depending on magnitude. Local to
// the cards/ subtree because the table view formats memory
// differently and we don't want a single shared helper to drift in
// either direction.
export function formatMem(n?: number): string {
    if (!n || n <= 0) return "—";
    const gb = n / (1024 * 1024 * 1024);
    if (gb < 1) return `${Math.round(n / (1024 * 1024))} MB`;
    if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
    return `${gb.toFixed(gb >= 10 ? 0 : 1)} GB`;
}

// renderOSLabel picks the most informative label the agent reported.
// Returns a string for the simple cases and `null` when there's
// nothing to show (so the caller can render a Dim em-dash).
export function osLabel(h: Host): string | null {
    if (h.platform) {
        const v = h.platform_version ? ` ${h.platform_version}` : "";
        return `${h.platform}${v}`;
    }
    if (h.os) return h.os;
    return null;
}
