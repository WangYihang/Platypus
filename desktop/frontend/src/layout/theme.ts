// Single source of truth for the Vercel-inspired visual system.
// Every component reads colours from here rather than from inline
// style={{}} sprinkled across files; the Ant Design ConfigProvider
// applied at the app root picks these up too so Ant components match.

import type { ThemeConfig } from "antd";

// --- Palette ---------------------------------------------------------
// Pure neutral grays. Two surface tiers only: main (#000) and surface
// (#0a0a0a). Borders carry the structure; no shadows.
export const palette = {
    // Region backgrounds (kept as fields so AppShell / legacy callers stay valid).
    rail: "#000000",
    sidebar: "#0a0a0a",
    main: "#000000",
    detailRail: "#0a0a0a",

    // Surfaces & borders.
    surface: "#0a0a0a",
    surfaceHover: "#171717",
    border: "#262626",
    borderStrong: "#404040",

    // Text.
    textPrimary: "#fafafa",
    textSecondary: "#a1a1a1",
    textMuted: "#737373",

    // Accent (Vercel uses near-white for primary fills).
    accent: "#fafafa",
    accentFg: "#0a0a0a",

    // Status.
    success: "#0070f3",
    successDot: "#3ECF8E",
    danger: "#ee0000",
    warning: "#f5a623",
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

// Ant ConfigProvider token overrides. Scope is deliberately narrow —
// colour + border radius + density + per-component tweaks for the
// surfaces we touch (Tabs, Tables, Buttons, Inputs).
export const antTheme: ThemeConfig = {
    token: {
        colorPrimary: palette.accent,
        colorInfo: palette.success,
        colorSuccess: palette.successDot,
        colorError: palette.danger,
        colorWarning: palette.warning,
        colorBgBase: palette.main,
        colorBgContainer: palette.main,
        colorBgElevated: palette.surface,
        colorTextBase: palette.textPrimary,
        colorBorder: palette.border,
        colorBorderSecondary: palette.border,
        borderRadius: radius.md,
        controlHeight: 32,
        fontFamily: font.sans,
    },
    components: {
        Layout: {
            bodyBg: palette.main,
            headerBg: palette.rail,
            siderBg: palette.sidebar,
        },
        Menu: {
            darkItemBg: palette.sidebar,
            darkItemSelectedBg: palette.surfaceHover,
        },
        Table: {
            headerBg: palette.surface,
            headerColor: palette.textSecondary,
            rowHoverBg: palette.surfaceHover,
            borderColor: palette.border,
        },
        Button: {
            defaultBg: "transparent",
            defaultBorderColor: palette.border,
            defaultColor: palette.textPrimary,
            primaryColor: palette.accentFg,
        },
        Input: {
            activeBorderColor: palette.borderStrong,
            hoverBorderColor: palette.borderStrong,
            colorBgContainer: palette.main,
        },
        Tabs: {
            itemColor: palette.textSecondary,
            itemSelectedColor: palette.textPrimary,
            itemHoverColor: palette.textPrimary,
            inkBarColor: palette.textPrimary,
        },
        Modal: {
            contentBg: palette.surface,
            headerBg: palette.surface,
        },
        Card: {
            colorBgContainer: palette.surface,
        },
        Select: {
            optionSelectedBg: palette.surfaceHover,
        },
    },
};
