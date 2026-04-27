// shouldSkipBrowserShortcut decides whether the FileBrowser's global
// keyboard listener should bow out for a particular keystroke.
//
// Two skip reasons:
//
//  1. The user is typing inside a real text-input control —
//     <input>, <textarea>, anything contenteditable, anything with
//     role="textbox", or any descendant of a CodeMirror editor (the
//     editor renders into a contenteditable wrapped under
//     `.cm-editor` / `.cm-content`).
//
//  2. The file editor is open at all. None of the FileBrowser's
//     shortcuts (Backspace = up a directory, F2 = rename,
//     Delete = delete selection, Ctrl/Cmd+N = new folder,
//     Enter = open) make sense while the operator is editing a
//     file, and even a stray focus on the surrounding document
//     should not surface Backspace as "navigate up" while the
//     editor is mounted.
//
// The previous implementation only checked `<input>, <textarea>`,
// which let CodeMirror's contenteditable through. Operators
// reported the editor closing on Backspace, with the FileBrowser
// then jumping up a level — exactly what this guard prevents.
export function shouldSkipBrowserShortcut(
    target: EventTarget | null,
    hasOpenEditor: boolean,
): boolean {
    if (hasOpenEditor) return true;
    if (!target || !(target instanceof Element)) return false;
    if (target.matches("input, textarea")) return true;
    if (
        target.matches(
            '[contenteditable], [contenteditable="true"], [role="textbox"]',
        )
    ) {
        return true;
    }
    if (target.closest(".cm-editor, .cm-content, [contenteditable]")) {
        return true;
    }
    return false;
}
