import { useCallback, useMemo } from "react";

import { usePreference } from "../../../lib/preferences";

// usePreviewPane is the storage layer behind the Quick-Look style
// preview pane. It tracks open/closed state only — the split width
// between the file list and the preview is owned by the
// ResizablePanelGroup in FileBrowser (see autoSaveId="files-preview-split"
// in localStorage), so we don't duplicate the persistence here.
//
// Backed by `ui.files.previewOpen` in the typed preference registry —
// previously a hand-rolled `files.previewOpen` localStorage key.

export interface PreviewPane {
    open: boolean;
    setOpen: (open: boolean) => void;
    toggle: () => void;
    close: () => void;
}

export function usePreviewPane(): PreviewPane {
    const [open, setOpen] = usePreference("ui.files.previewOpen");
    const toggle = useCallback(() => setOpen(!open), [open, setOpen]);
    const close = useCallback(() => setOpen(false), [setOpen]);
    // Memoised for the same reason as useDirectory: consumers put this
    // object in dependency arrays, and a fresh literal per render makes
    // every one of them re-run forever.
    return useMemo(
        () => ({ open, setOpen, toggle, close }),
        [open, setOpen, toggle, close],
    );
}
