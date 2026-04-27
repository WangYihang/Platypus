import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import CmdKHint from "./CmdKHint";

// CmdKHint surfaces the keyboard shortcut for the global command
// palette as a small clickable kbd-styled badge. Without this, new
// operators have no way to discover Ctrl/Cmd+K — the binding is
// alive at the shell level but invisible.
//
// Contract:
//   1. The badge displays a "K" with a meta/control prefix glyph.
//   2. Clicking the badge dispatches the same keydown event the
//      palette listens for, so the click and the shortcut share
//      a single open path.

describe("<CmdKHint>", () => {
    it("renders a K shortcut hint", () => {
        render(<CmdKHint />);
        const badge = screen.getByRole("button", { name: /command palette/i });
        expect(badge).toBeInTheDocument();
        // The badge text mentions K so it reads as a shortcut, not
        // a generic icon.
        expect(badge.textContent).toMatch(/k/i);
    });

    it("dispatches a Cmd/Ctrl+K keydown event when clicked", () => {
        const events: KeyboardEvent[] = [];
        const listener = (e: Event) => events.push(e as KeyboardEvent);
        window.addEventListener("keydown", listener);

        render(<CmdKHint />);
        fireEvent.click(screen.getByRole("button", { name: /command palette/i }));

        const k = events.find((e) => e.key.toLowerCase() === "k");
        expect(k).toBeDefined();
        // Either ctrl OR meta — the palette listens for either.
        expect(k!.ctrlKey || k!.metaKey).toBe(true);
        window.removeEventListener("keydown", listener);
    });
});
