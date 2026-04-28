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
    return (
        <div className="h-full">
            <FileBrowser
                projectID={projectID}
                sessionHash={sessionHash}
                host={host}
            />
        </div>
    );
}
