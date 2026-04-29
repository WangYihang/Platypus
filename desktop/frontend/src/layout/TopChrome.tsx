import { ReactNode } from "react";
import { Search } from "lucide-react";

import { palette, radius, space } from "./theme";

// TopChrome is the global top bar that sits above the
// rail + main split and below the OS chrome. Three zones:
//
//   [ left slot ]   [ centered Cmd-K trigger ]   [ right slot ]
//
// The Cmd-K trigger is rendered as a styled-input button — wide,
// centred, with a lucide search icon on the left and a `⌘K` /
// `Ctrl+K` keycap chip on the right. Click dispatches the same
// keydown the existing CommandPalette listens for, so a click and
// the shortcut go through the same open path.
//
// Left and right slots are caller-controlled and stay narrow on
// purpose so the centered trigger remains visually anchored.

interface Props {
    left?: ReactNode;
    right?: ReactNode;
}

function isMac(): boolean {
    return (
        typeof navigator !== "undefined" &&
        !!navigator.platform &&
        /Mac/i.test(navigator.platform)
    );
}

function triggerCmdK() {
    const evt = new KeyboardEvent("keydown", {
        key: "k",
        ctrlKey: true,
        metaKey: true,
        bubbles: true,
        cancelable: true,
    });
    window.dispatchEvent(evt);
}

export default function TopChrome({ left, right }: Props) {
    const meta = isMac() ? "⌘" : "Ctrl";
    return (
        <div
            data-testid="top-chrome"
            style={{
                flexShrink: 0,
                height: 44,
                display: "flex",
                alignItems: "center",
                gap: space[3],
                padding: `0 ${space[3]}px`,
                background: palette.rail,
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <div
                style={{
                    minWidth: 0,
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    flexShrink: 0,
                }}
            >
                {left}
            </div>
            <div
                style={{
                    flex: 1,
                    minWidth: 0,
                    display: "flex",
                    justifyContent: "center",
                }}
            >
                <button
                    type="button"
                    onClick={triggerCmdK}
                    data-testid="top-chrome-cmdk"
                    aria-label="Open command palette"
                    title={`Open command palette (${meta}+K)`}
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: space[2],
                        width: "100%",
                        maxWidth: 560,
                        height: 28,
                        padding: `0 ${space[2]}px 0 10px`,
                        background: palette.surface,
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.md,
                        color: palette.textMuted,
                        fontFamily: "var(--font-geist-mono)",
                        fontSize: 12,
                        cursor: "pointer",
                        textAlign: "left",
                    }}
                >
                    <Search
                        className="size-3.5"
                        style={{ color: palette.textMuted, flexShrink: 0 }}
                    />
                    <span style={{ flex: 1, minWidth: 0 }}>
                        Search or run command…
                    </span>
                    <span
                        aria-hidden
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 2,
                            padding: "1px 6px",
                            background: palette.main,
                            border: `1px solid ${palette.border}`,
                            borderRadius: radius.sm,
                            color: palette.textMuted,
                            fontSize: 10,
                            lineHeight: 1.4,
                            flexShrink: 0,
                        }}
                    >
                        {meta}K
                    </span>
                </button>
            </div>
            <div
                style={{
                    minWidth: 0,
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    flexShrink: 0,
                }}
            >
                {right}
            </div>
        </div>
    );
}
