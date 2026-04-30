import { ReactNode, useEffect } from "react";
import { ChevronDown, ChevronUp, X } from "lucide-react";

import { palette, space } from "../../layout/theme";

export type BottomTab = "processes" | "tunnels";

interface Props {
    open: boolean;
    activeTab: BottomTab;
    onActiveTabChange: (tab: BottomTab) => void;
    onToggle: () => void;
    onClose: () => void;
    height: number;
    onHeightChange: (px: number) => void;
    children: ReactNode;
}

const HEADER_PX = 28;
const MIN_PX = 120;
const MAX_FRACTION = 0.7;

// BottomPanel is the VSCode-style "peek-while-editing" surface inside
// HostView. Default collapsed; click the header or press a future
// keybind to expand. Tabs inside it (Processes, Tunnels) mirror the
// activities of the same name — same data, but framed for a quick
// glance rather than a full-pane focus.
//
// We deliberately do NOT host a Terminal tab here; the existing
// global TerminalDrawer (mounted at the shell level via
// GlobalTerminalContext) keeps owning the xterm session lifecycle so
// shell state survives across host / activity switches without a
// duplicate xterm instance fighting for the WebSocket.
export default function BottomPanel({
    open,
    activeTab,
    onActiveTabChange,
    onToggle,
    onClose,
    height,
    onHeightChange,
    children,
}: Props) {
    // Clamp the panel height to a comfortable range so dragging the
    // splitter can't shrink it below the header strip or eat the
    // ActivityPane entirely.
    useEffect(() => {
        const max = Math.max(MIN_PX, window.innerHeight * MAX_FRACTION);
        if (open && height < MIN_PX) onHeightChange(MIN_PX);
        if (open && height > max) onHeightChange(max);
    }, [open, height, onHeightChange]);

    const headerRow = (
        <div
            data-testid="host-bottom-panel-header"
            style={{
                flexShrink: 0,
                display: "flex",
                alignItems: "center",
                gap: space[2],
                height: HEADER_PX,
                padding: `0 ${space[3]}px`,
                borderTop: `1px solid ${palette.border}`,
                borderBottom: open ? `1px solid ${palette.border}` : "none",
                background: palette.rail,
                fontSize: 11,
                color: palette.textMuted,
            }}
        >
            <PanelTab
                label="Processes"
                active={open && activeTab === "processes"}
                onClick={() => {
                    if (!open) onToggle();
                    onActiveTabChange("processes");
                }}
            />
            <PanelTab
                label="Tunnels"
                active={open && activeTab === "tunnels"}
                onClick={() => {
                    if (!open) onToggle();
                    onActiveTabChange("tunnels");
                }}
            />
            <span style={{ flex: 1 }} />
            <button
                type="button"
                aria-label={open ? "Collapse panel" : "Expand panel"}
                onClick={onToggle}
                title={open ? "Collapse" : "Expand"}
                style={iconBtnStyle}
            >
                {open ? <ChevronDown className="size-3.5" /> : <ChevronUp className="size-3.5" />}
            </button>
            {open && (
                <button
                    type="button"
                    aria-label="Close panel"
                    onClick={onClose}
                    title="Close panel"
                    style={iconBtnStyle}
                >
                    <X className="size-3.5" />
                </button>
            )}
        </div>
    );

    if (!open) {
        return (
            <div data-testid="host-bottom-panel" style={{ flexShrink: 0 }}>
                {headerRow}
            </div>
        );
    }

    return (
        <div
            data-testid="host-bottom-panel"
            style={{
                flexShrink: 0,
                height,
                display: "flex",
                flexDirection: "column",
                background: palette.surface,
            }}
        >
            {headerRow}
            <div
                style={{
                    flex: 1,
                    minHeight: 0,
                    overflow: "auto",
                }}
            >
                {children}
            </div>
        </div>
    );
}

function PanelTab({
    label,
    active,
    onClick,
}: {
    label: string;
    active: boolean;
    onClick: () => void;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            data-active={active || undefined}
            style={{
                background: "transparent",
                border: "none",
                padding: "4px 8px",
                cursor: "pointer",
                fontSize: 11,
                fontWeight: active ? 600 : 500,
                color: active ? palette.textPrimary : palette.textMuted,
                borderBottom: active ? `1px solid ${palette.accent}` : "1px solid transparent",
                marginBottom: -1,
                lineHeight: 1.5,
            }}
        >
            {label}
        </button>
    );
}

const iconBtnStyle: React.CSSProperties = {
    background: "transparent",
    border: "none",
    cursor: "pointer",
    color: palette.textMuted,
    width: 20,
    height: 20,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 4,
};
