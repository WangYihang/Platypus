import { useCallback, useEffect, useState } from "react";

// usePreviewPane is the storage layer behind the Quick-Look style
// preview pane. It tracks open/closed state only — the split width
// between the file list and the preview is owned by the
// ResizablePanelGroup in FileBrowser (see autoSaveId="files-preview-split"
// in localStorage), so we don't duplicate the persistence here.
//
// Persistence is intentionally per-browser (localStorage), not
// per-server: the user's preferred layout follows them across project
// switches but doesn't roam to other devices.

const KEY_OPEN = "files.previewOpen";

function loadOpen(): boolean {
    if (typeof window === "undefined") return false;
    try {
        return window.localStorage.getItem(KEY_OPEN) === "true";
    } catch {
        return false;
    }
}

export interface PreviewPane {
    open: boolean;
    setOpen: (open: boolean) => void;
    toggle: () => void;
    close: () => void;
}

export function usePreviewPane(): PreviewPane {
    const [open, setOpenRaw] = useState<boolean>(loadOpen);

    useEffect(() => {
        try {
            window.localStorage.setItem(KEY_OPEN, String(open));
        } catch {
            // Quota errors etc. are best-effort; preview state is
            // not critical so we silently lose persistence.
        }
    }, [open]);

    const setOpen = useCallback((next: boolean) => {
        setOpenRaw(next);
    }, []);

    const toggle = useCallback(() => setOpenRaw((o) => !o), []);
    const close = useCallback(() => setOpenRaw(false), []);

    return { open, setOpen, toggle, close };
}
