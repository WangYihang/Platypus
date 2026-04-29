// Single source of truth for the Vercel-inspired visual system.
//
// Two kinds of consumers read these tokens:
//   · inline-style React (palette.textMuted, space[4], radius.md, …).
//     The transitional files that haven't been rewritten to Tailwind
//     classes still use this TS-side mirror.
//   · shadcn/ui + Tailwind utilities. style.css hoists the same values
//     into CSS variables (:root) and Tailwind theme keys (@theme inline),
//     so `bg-surface`, `text-text-muted`, `rounded-md`, `bg-primary`,
//     etc. resolve to the same numbers / hex codes you see below.
//
// Keep this file and the :root / @theme blocks in style.css in sync —
// change values here, update style.css verbatim, and rebuild.

// --- Palette ---------------------------------------------------------
// Vercel-style neutral grays. Three visible surface tiers — main (the
// content background, slightly lighter than rail/sidebar so the focus
// area pops forward) and surface (cards on top of main). Rail + sidebar
// share a darker background so the chrome reads as separate from the
// content. Borders carry the structure; no shadows.
export const palette = {
    // Region backgrounds.
    rail: "#0a0a0a",
    sidebar: "#0a0a0a",
    main: "#0d0d0d",
    detailRail: "#0a0a0a",

    // Surfaces & borders. main / surface used to be #111 vs #1a1a1a,
    // only 9 RGB units apart and effectively indistinguishable in
    // dark mode. Keep these in sync with --color-main / --color-surface
    // in style.css; the spec at e2e/specs/50-surface-vs-main-contrast
    // pins the channel delta at >= 12 so the regression can't sneak
    // back in.
    surface: "#1f1f1f",
    surfaceHover: "#262626",
    border: "#2e2e2e",
    borderStrong: "#525252",

    // Text.
    textPrimary: "#fafafa",
    textSecondary: "#a1a1a1",
    textMuted: "#737373",

    // Accent (Vercel uses near-white for primary fills).
    accent: "#fafafa",
    accentFg: "#0a0a0a",

    // Status / intent.
    //
    // Names follow what the user reads: success = green (operation
    // succeeded), info = blue (informational accent / focus rings /
    // "this finished" pills), warning = amber, danger = red. The
    // previous tokens success (blue) / successDot (green) had it
    // backwards — surfaces showed a blue "Pending" pill that read
    // as success while the actual success colour hid behind a
    // -dot suffix.
    success: "#3ECF8E",
    info: "#0070f3",
    // infoSoft is the translucent companion of info, used by the
    // status-bar pills to "light up" without changing layout when a
    // count > 0 (operator: "太不显眼"). Same hue, ~12% alpha, sits
    // happily over the rail without bleeding into adjacent text.
    infoSoft: "rgba(0, 112, 243, 0.18)",
    danger: "#ee0000",
    warning: "#f5a623",

    // Server-rail tile backgrounds. One stable bucket per URL via
    // avatarBg() in lib/servers.ts; the letter sits on top. Ten
    // colours keeps collision density low even at the 16-profile cap
    // without turning the rail into a rainbow.
    avatarBgs: [
        "#3b82f6", "#8b5cf6", "#ec4899", "#ef4444", "#f97316",
        "#eab308", "#22c55e", "#14b8a6", "#06b6d4", "#6366f1",
    ] as const,
} as const;

// Spacing / radius / font tokens. Components consume these instead of
// magic numbers so density tweaks land in one place.
export const radius = { sm: 4, md: 6, lg: 8, pill: 9999 } as const;
export const space = { 1: 4, 2: 8, 3: 12, 4: 16, 5: 20, 6: 24, 8: 32 } as const;
// `font.sans` aliased to the mono family — every UI surface now
// renders in Geist Mono per the Round-3 redesign. The two keys exist
// only so existing inline-style consumers don't have to be migrated
// one-by-one; Phase 6 deletes the `sans` export and rewrites those
// callers to use plain Tailwind classes.
export const font = {
    sans: "var(--font-geist-mono)",
    mono: "var(--font-geist-mono)",
} as const;

// Region widths. Kept here so CSS transitions can read them and the
// collapsed states animate predictably.
//
// 2026-04 density pass: the original widths were sized for a 1080p
// laptop with a sparse nav. On 1440p+ the 280-wide sidebar plus the
// 56-tall page header pushed the file list to ~30% of the viewport
// before the operator could see a single row. Tighter defaults bring
// that back without forcing every consumer to opt in via a density
// toggle. The ResizablePanel still lets the user widen the sidebar
// if their monitor permits. `profileRailWidth` was retired when the
// standalone ServerRail column was folded into ProjectSidebar's
// header (now ServerSwitcher.tsx).
export const layout = {
    sidebarWidth: 220,
    detailRailWidth: 220,
    mainHeaderHeight: 40,
} as const;
