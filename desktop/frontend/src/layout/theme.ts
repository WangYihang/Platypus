// Single source of truth for the new Slack-inspired visual system.
// Every component reads colours from here rather than from inline
// style={{}} sprinkled across files; the Ant Design ConfigProvider
// applied at the app root picks these up too so Ant components match.

import type { ThemeConfig } from "antd";

// --- Palette ---------------------------------------------------------
// Region backgrounds go from darkest (profile rail) to slightly lighter
// (main panel) so the eye has a natural focus gradient.
export const palette = {
    rail: "#19171d",       // profile rail (leftmost narrow strip)
    sidebar: "#1a1d29",    // project/host/listener list
    main: "#1e2130",       // main content panel
    detailRail: "#1a1d29", // right-side metadata rail
    border: "#2a2e3d",     // 1px dividers between regions
    textPrimary: "#e8e8e8",
    textSecondary: "#8a8f9c",
    accent: "#4a9eff",     // selection, primary buttons, links
    success: "#2bac76",    // online presence dot, success tags
    danger: "#e5484d",     // destructive actions, root user tag
    warning: "#f5a623",
} as const;

// Region widths. Kept here so CSS transitions can read them and the
// collapsed states animate predictably.
export const layout = {
    profileRailWidth: 56,
    sidebarWidth: 280,
    detailRailWidth: 280,
    mainHeaderHeight: 48,
} as const;

// Ant ConfigProvider token overrides. Scope is deliberately narrow —
// colour + border radius + density. Anything more opinionated (menus,
// modals) should be composed in the components, not globally themed.
export const antTheme: ThemeConfig = {
    token: {
        colorPrimary: palette.accent,
        colorInfo: palette.accent,
        colorSuccess: palette.success,
        colorError: palette.danger,
        colorWarning: palette.warning,
        colorBgBase: palette.main,
        colorTextBase: palette.textPrimary,
        colorBorder: palette.border,
        borderRadius: 6,
        fontFamily:
            '"Nunito", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto",\n' +
            '"Oxygen", "Ubuntu", "Cantarell", "Fira Sans", "Droid Sans",\n' +
            '"Helvetica Neue", sans-serif',
    },
    components: {
        Layout: {
            bodyBg: palette.main,
            headerBg: palette.rail,
            siderBg: palette.sidebar,
        },
        Menu: {
            darkItemBg: palette.sidebar,
            darkItemSelectedBg: palette.accent,
        },
        Table: {
            headerBg: palette.sidebar,
        },
    },
};
