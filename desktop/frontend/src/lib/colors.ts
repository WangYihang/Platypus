// colors.ts is the deterministic id-to-palette helper. Used by the
// server rail, the terminal drawer (host indicator), and the status-
// bar terminals popover so the same machine_id always renders with
// the same accent — operators learn the mapping after a few sessions
// and can identify which host a tab belongs to at a glance.
//
// Backed by palette.avatarBgs so the colour set is curated; when we
// outgrow ten buckets the registry can expand without touching call
// sites.

import { palette } from "../layout/theme";

const PALETTE = palette.avatarBgs;

// Simple FNV-1a 32-bit hash. We don't need cryptographic strength —
// we want stability across reloads and a reasonable distribution
// over a couple of dozen ids.
function hash32(s: string): number {
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) {
        h ^= s.charCodeAt(i);
        // Multiply by FNV prime and clamp to 32-bit.
        h = Math.imul(h, 0x01000193);
    }
    return h >>> 0;
}

export function colorForId(id: string | null | undefined): string {
    if (!id) return PALETTE[0];
    return PALETTE[hash32(id) % PALETTE.length];
}

// contrastingTextColor returns "#ffffff" or "#000000" depending on
// which reads better against `bg`. Implements the WCAG-recommended
// per-channel relative luminance approximation; operators see the
// dot+text combo on the dark Platypus chrome AND on a coloured
// background (e.g. status-bar pills), so matching this is what keeps
// the labels legible.
export function contrastingTextColor(bg: string): string {
    const m = /^#?([0-9a-f]{6})$/i.exec(bg.trim());
    if (!m) return "#ffffff";
    const v = parseInt(m[1], 16);
    const r = (v >> 16) & 0xff;
    const g = (v >> 8) & 0xff;
    const b = v & 0xff;
    // Quick perceptual luminance — Rec. 601 weights are good enough.
    const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    return luminance > 0.55 ? "#000000" : "#ffffff";
}
