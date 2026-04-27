import { describe, expect, it } from "vitest";

import { shouldSkipBrowserShortcut } from "./keymap";

// shouldSkipBrowserShortcut decides whether the FileBrowser's
// global keyboard listener (Backspace=up, F2=rename, Delete=delete,
// Ctrl+N=new folder, Enter=open) should bow out for a particular
// keystroke. It used to only skip plain <input>/<textarea>; that
// missed CodeMirror — which renders into a contenteditable, not a
// textarea — so typing Backspace inside an open file editor would
// close the editor and pop the directory back up one level.
//
// Two skip reasons:
//   1. The keystroke is happening inside a real text-input control
//      (input / textarea / contenteditable / role=textbox / inside
//      a CodeMirror editor).
//   2. The browser-level file editor is open at all — none of the
//      shortcuts make sense while the operator is editing a file,
//      and even tabbing focus out of the editor shouldn't expose
//      Backspace = navigate-up.

function el(tag: string, attrs: Record<string, string> = {}): HTMLElement {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
        node.setAttribute(k, v);
    }
    return node;
}

describe("shouldSkipBrowserShortcut", () => {
    it("skips for <input>", () => {
        const target = el("input");
        expect(shouldSkipBrowserShortcut(target, false)).toBe(true);
    });

    it("skips for <textarea>", () => {
        const target = el("textarea");
        expect(shouldSkipBrowserShortcut(target, false)).toBe(true);
    });

    it("skips for contenteditable elements", () => {
        const target = el("div", { contenteditable: "true" });
        expect(shouldSkipBrowserShortcut(target, false)).toBe(true);
    });

    it("skips for role=textbox", () => {
        const target = el("div", { role: "textbox" });
        expect(shouldSkipBrowserShortcut(target, false)).toBe(true);
    });

    it("skips for descendants of a CodeMirror editor", () => {
        const editor = el("div", { class: "cm-editor" });
        const inner = el("span");
        editor.appendChild(inner);
        document.body.appendChild(editor);
        expect(shouldSkipBrowserShortcut(inner, false)).toBe(true);
        document.body.removeChild(editor);
    });

    it("does NOT skip for a plain div", () => {
        const target = el("div");
        expect(shouldSkipBrowserShortcut(target, false)).toBe(false);
    });

    it("skips for ANYTHING when an editor is open", () => {
        const target = el("div");
        expect(shouldSkipBrowserShortcut(target, true)).toBe(true);
    });

    it("returns false for null target without an open editor", () => {
        expect(shouldSkipBrowserShortcut(null, false)).toBe(false);
    });
});
