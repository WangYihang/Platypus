import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { palette, radius, space } from "../../../layout/theme";

import {
    ARCHIVE_FORMATS,
    ArchiveFormat,
    archiveLabel,
    suggestedArchiveFilename,
} from "./archive";

interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    // Names of the entries being archived. Single folder → ["nginx"];
    // mixed selection → ["nginx", "config.json", ...]. The dialog
    // shows the names back so the user confirms what's being packed.
    names: string[];
    onConfirm: (format: ArchiveFormat) => void | Promise<void>;
}

// FolderArchiveDialog is the modal that appears when a download
// includes one or more folders. The native browser save dialog can
// only accept a single file, so we have to package the folder(s)
// before that dialog opens. Format choice happens here:
//
//   tar.gz  — most common Linux operator pick (default)
//   tar     — uncompressed, faster for already-compressed payloads
//   zip     — cross-platform / Windows-friendly
//
// On confirm we hand the chosen format up to FileBrowser, which then
// asks the OS for a save location and streams the archive in chunks
// (the actual archive build is server-side).
export default function FolderArchiveDialog({
    open,
    onOpenChange,
    names,
    onConfirm,
}: Props) {
    const [format, setFormat] = useState<ArchiveFormat>("tar.gz");
    const [submitting, setSubmitting] = useState(false);

    // Reset to tar.gz every time the dialog re-opens so the choice
    // doesn't sticky-leak across unrelated downloads.
    useEffect(() => {
        if (open) {
            setFormat("tar.gz");
            setSubmitting(false);
        }
    }, [open]);

    async function handleConfirm() {
        setSubmitting(true);
        try {
            await onConfirm(format);
        } finally {
            setSubmitting(false);
        }
    }

    const filename = suggestedArchiveFilename(names, format);

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[440px]">
                <DialogHeader>
                    <DialogTitle>Download as archive</DialogTitle>
                    <DialogDescription>
                        Folders can't be saved through the OS file picker
                        directly — pick how to package the selection.
                    </DialogDescription>
                </DialogHeader>
                <div
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                        paddingTop: space[1],
                    }}
                >
                    <div style={{ fontSize: 12, color: palette.textMuted }}>
                        Packaging:
                    </div>
                    <div
                        style={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: space[1],
                            border: `1px solid ${palette.border}`,
                            borderRadius: radius.sm,
                            padding: `${space[2]}px ${space[3]}px`,
                            maxHeight: 100,
                            overflowY: "auto",
                            fontSize: 12,
                            fontFamily: "var(--font-geist-mono)",
                            color: palette.textPrimary,
                        }}
                    >
                        {names.map((n) => (
                            <span
                                key={n}
                                style={{
                                    background: palette.surface,
                                    border: `1px solid ${palette.border}`,
                                    borderRadius: radius.sm,
                                    padding: `1px ${space[2]}px`,
                                }}
                            >
                                {n}
                            </span>
                        ))}
                    </div>
                    <fieldset
                        style={{
                            display: "flex",
                            flexDirection: "column",
                            gap: space[2],
                            border: 0,
                            margin: 0,
                            padding: 0,
                        }}
                    >
                        <legend
                            style={{
                                fontSize: 12,
                                color: palette.textMuted,
                                marginBottom: space[1],
                            }}
                        >
                            Archive format
                        </legend>
                        {ARCHIVE_FORMATS.map((f) => (
                            <label
                                key={f}
                                style={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: space[2],
                                    padding: `${space[2]}px ${space[3]}px`,
                                    border: `1px solid ${
                                        format === f ? palette.borderStrong : palette.border
                                    }`,
                                    borderRadius: radius.sm,
                                    cursor: "pointer",
                                    fontSize: 13,
                                    color: palette.textPrimary,
                                    background:
                                        format === f
                                            ? palette.surfaceHover
                                            : "transparent",
                                }}
                            >
                                <input
                                    type="radio"
                                    name="archive-format"
                                    value={f}
                                    checked={format === f}
                                    onChange={() => setFormat(f)}
                                    // The accessible name is just the
                                    // bare format key — tests can match
                                    // /^tar.gz$/ without colliding with
                                    // the "gzip-compressed" copy in the
                                    // visible label.
                                    aria-label={f}
                                />
                                <span>{archiveLabel(f)}</span>
                            </label>
                        ))}
                    </fieldset>
                    <div
                        style={{
                            fontSize: 11,
                            color: palette.textMuted,
                            fontFamily: "var(--font-geist-mono)",
                        }}
                    >
                        Saved as <span style={{ color: palette.textSecondary }}>{filename}</span>
                    </div>
                </div>
                <DialogFooter>
                    <Button
                        type="button"
                        variant="outline"
                        onClick={() => onOpenChange(false)}
                        disabled={submitting}
                    >
                        Cancel
                    </Button>
                    <Button type="button" onClick={handleConfirm} disabled={submitting}>
                        {submitting && <Loader2 className="size-3.5 animate-spin" />}
                        Download
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
