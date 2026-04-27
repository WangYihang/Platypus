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
export const font = {
    sans: "var(--font-geist-sans)",
    mono: "var(--font-geist-mono)",
} as const;

// Region widths. Kept here so CSS transitions can read them and the
// collapsed states animate predictably.
export const layout = {
    profileRailWidth: 56,
    sidebarWidth: 280,
    detailRailWidth: 280,
    mainHeaderHeight: 56,
} as const;
