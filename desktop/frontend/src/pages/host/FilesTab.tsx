import type { Host } from "../../lib/api";
import FileBrowser from "./files/FileBrowser";

// FilesTab is the per-session file-management panel embedded in
// HostView. It renders a full file explorer — breadcrumb navigation, a
// sortable table with multi-select + keyboard, drag-drop upload from
// the OS, internal drag-to-move via Rename, inline CodeMirror editing
// for files under 5 MiB, and a paged read-only viewer for larger ones.
//
// `host` is forwarded so FileBrowser's QuickPaths chip row can pick
// platform-appropriate roots (Linux: /, ~, /etc, …; Windows: C:\, …).

interface Props {
    projectID: string;
    sessionHash: string;
    host: Host | null;
}

export default function FilesTab({ projectID, sessionHash, host }: Props) {
    // HostView wraps every tab in 24px of outer padding so the Info /
    // Sessions / Processes cards breathe. The Files tab has no card
    // chrome of its own — it's a full-bleed file browser — and that
    // 24px gutter visibly chops the available rows on common laptop
    // viewports. Pull half of it back with a negative margin so the
    // browser can stretch to the panel edges without disturbing the
    // other tabs' layouts. flex+min-h-0 makes the browser share the
    // remaining viewport with the bottom terminal drawer — when the
    // drawer grows, this region shrinks instead of forcing scroll.
    return (
        <div className="-mx-3 -my-3 flex min-h-0 w-auto flex-1 flex-col">
            <FileBrowser
                projectID={projectID}
                sessionHash={sessionHash}
                host={host}
            />
        </div>
    );
}
