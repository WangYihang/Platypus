import { palette, radius } from "./theme";

// CmdKHint is the discoverable affordance for the keyboard-first
// command palette. The actual binding (Cmd/Ctrl+K → open palette)
// lives at the shell level in CommandPalette; this component is the
// visible shortcut hint that operators new to Platypus can spot
// without having to learn the keybinding from documentation.
//
// Click behaviour: dispatches the same keydown event the palette
// listens for, so a click is functionally equivalent to typing the
// shortcut. Single open path, single test surface.
export default function CmdKHint() {
    const isMac =
        typeof navigator !== "undefined" &&
        navigator.platform &&
        /Mac/i.test(navigator.platform);
    const meta = isMac ? "⌘" : "Ctrl";

    function trigger() {
        const evt = new KeyboardEvent("keydown", {
            key: "k",
            // Set both flags so the listener fires on either platform.
            ctrlKey: true,
            metaKey: true,
            bubbles: true,
            cancelable: true,
        });
        window.dispatchEvent(evt);
    }

    return (
        <button
            type="button"
            onClick={trigger}
            aria-label="Open command palette"
            title={`Open command palette (${meta}+K)`}
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 3,
                padding: "1px 6px",
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.sm,
                color: palette.textMuted,
                fontFamily: "var(--font-geist-mono)",
                fontSize: 10,
                lineHeight: 1.4,
                cursor: "pointer",
            }}
        >
            <span aria-hidden>{meta}</span>
            <span aria-hidden>+</span>
            <span>K</span>
        </button>
    );
}
